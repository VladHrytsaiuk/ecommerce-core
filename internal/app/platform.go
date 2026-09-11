package app

import (
	"context"
	"fmt"
	"io"

	platformCache "github.com/VladHrytsaiuk/ecommerce-core/internal/platform/cache"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/config"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/observability"
	platformRedis "github.com/VladHrytsaiuk/ecommerce-core/internal/platform/redis"
	sharedCache "github.com/VladHrytsaiuk/ecommerce-core/internal/shared/cache"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/ratelimit"
)

// platformRuntime is the infrastructure every module draws on: caching, shared
// rate limiting and telemetry. It exists so the Composition Root can acquire
// these together, and so that failing halfway through acquisition releases
// what it already opened instead of leaking a Redis pool and a trace exporter
// into a process that is about to exit.
type platformRuntime struct {
	Cache        sharedCache.Service
	LoginLimiter ratelimit.Service
	// Redis is nil when the capability is disabled. It is exposed only so the
	// management server can report its health; modules receive ports.
	Redis             *platformRedis.Client
	closers           []io.Closer
	telemetryShutdown func(context.Context) error
	committed         bool
}

// newPlatformRuntime acquires infrastructure in dependency order.
//
// Redis is opt-in. With it disabled the no-op cache and the per-process
// limiter keep a constrained deployment fully functional, which is why neither
// is an error path.
func newPlatformRuntime(cfg *config.Config, storeConfig StoreConfig) (*platformRuntime, error) {
	runtime := &platformRuntime{
		Cache:        sharedCache.NewNoOpService(),
		LoginLimiter: ratelimit.NewLocalService(),
	}

	telemetryShutdown, err := observability.Init(context.Background(), observability.Config{
		Enabled:     cfg.OTelEnabled,
		Endpoint:    cfg.OTelEndpoint,
		ServiceName: "ecommerce-core",
		Environment: cfg.Env,
	})
	if err != nil {
		return nil, fmt.Errorf("configure observability: %w", err)
	}
	runtime.telemetryShutdown = telemetryShutdown

	if cfg.RedisEnabled {
		client, redisErr := platformRedis.Connect(context.Background(), cfg.RedisURL)
		if redisErr != nil {
			// Telemetry is already open at this point, so the caller's rollback
			// has to run even though the runtime is never returned.
			runtime.rollback()
			return nil, fmt.Errorf("configure Redis: %w", redisErr)
		}
		runtime.closers = append(runtime.closers, client)
		runtime.Redis = client
		runtime.Cache = platformCache.NewRedisAdapter(client.Raw(), storeConfig.Code+":")
		runtime.LoginLimiter = platformRedis.NewFixedWindowLimiter(client.Raw())
	}
	return runtime, nil
}

// commit marks the runtime as owned by a successfully assembled Application,
// after which releasing it is the Application's shutdown responsibility.
func (p *platformRuntime) commit() { p.committed = true }

// rollback releases everything acquired so far. It is a no-op once committed,
// so it can be deferred unconditionally by the Composition Root.
func (p *platformRuntime) rollback() {
	if p == nil || p.committed {
		return
	}
	for _, closer := range p.closers {
		_ = closer.Close()
	}
	if p.telemetryShutdown != nil {
		_ = p.telemetryShutdown(context.Background())
	}
	p.closers, p.telemetryShutdown = nil, nil
}

// adopt registers a resource opened later during composition, so a module that
// fails after acquiring one still has it released by the same rollback.
func (p *platformRuntime) adopt(closer io.Closer) {
	if p != nil && closer != nil {
		p.closers = append(p.closers, closer)
	}
}

// closer hands the Application a single handle for the pooled resources.
func (p *platformRuntime) closer() io.Closer {
	if p == nil || len(p.closers) == 0 {
		return nil
	}
	return closeAll(p.closers)
}
