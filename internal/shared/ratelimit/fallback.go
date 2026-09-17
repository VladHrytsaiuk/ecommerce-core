package ratelimit

import (
	"context"
	"sync/atomic"
	"time"
)

// Logger reports the transition into and out of degraded limiting. Only the
// transition: logging every request during an outage would bury the line that
// says an outage began.
type Logger interface {
	Warnw(string, ...interface{})
	Infow(string, ...interface{})
}

// Fallback keeps a rate limit in force when its primary store is unreachable.
//
// The distributed limiter is the one worth having: it counts a client across
// every replica. But an error from it refuses the request, and the same
// middleware guarded login and all public browsing — so a Redis blip returned
// 503 for the entire versioned API. Catalog, checkout and orders went down
// because a cache did, in a system whose own architecture notes call Redis "an
// optional performance and security capability, not a transactional
// integration boundary".
//
// Login does not use this: losing brute-force protection is worse than refusing
// the request, and that decision is deliberate and documented where it is made.
// Public browsing does, and degrades to a per-process window instead — with N
// replicas a client gets N times the limit, which is a far better outcome than
// the storefront returning 503.
type Fallback struct {
	primary  Service
	standby  Service
	logger   Logger
	degraded atomic.Bool
}

func NewFallback(primary, standby Service, logger Logger) *Fallback {
	return &Fallback{primary: primary, standby: standby, logger: logger}
}

func (f *Fallback) Allow(ctx context.Context, key string, limit int, window time.Duration) (Decision, error) {
	if f == nil || f.primary == nil || f.standby == nil {
		return Decision{}, errNotConfigured
	}
	decision, err := f.primary.Allow(ctx, key, limit, window)
	if err == nil {
		f.recovered()
		return decision, nil
	}
	f.degrade(err)
	return f.standby.Allow(ctx, key, limit, window)
}

// degrade announces the first failure after a healthy period. Subsequent
// failures are silent until the primary recovers, so an outage produces one
// line rather than one per request.
func (f *Fallback) degrade(cause error) {
	if f.degraded.Swap(true) || f.logger == nil {
		return
	}
	f.logger.Warnw("rate limiter degraded to per-process windows",
		"error", cause.Error(),
		"effect", "a client is limited per replica rather than across the fleet")
}

// recovered runs on the success path, which is every request to the versioned
// API. Load before Swap deliberately: Swap is a locked read-modify-write even
// when the value does not change, so writing false over false on every request
// bounced one shared cache line between every core serving traffic. The common
// case is a plain read that hits the local line.
func (f *Fallback) recovered() {
	if !f.degraded.Load() {
		return
	}
	if !f.degraded.Swap(false) || f.logger == nil {
		return
	}
	f.logger.Infow("rate limiter recovered; limits are distributed again")
}
