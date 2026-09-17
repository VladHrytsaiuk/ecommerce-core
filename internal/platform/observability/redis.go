package observability

import (
	"context"
	"net"

	redis "github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// RedisHook emits spans without recording command arguments, which can contain
// cache values, tokens, or other customer data.
type RedisHook struct{}

func NewRedisHook() RedisHook { return RedisHook{} }

func (RedisHook) DialHook(next redis.DialHook) redis.DialHook {
	return func(ctx context.Context, network, address string) (netConn net.Conn, err error) {
		ctx, span := otel.Tracer("ecommerce-core/redis").Start(ctx, "redis dial", trace.WithSpanKind(trace.SpanKindClient), trace.WithAttributes(attribute.String("network.transport", network)))
		defer span.End()
		netConn, err = next(ctx, network, address)
		if err != nil {
			span.RecordError(err)
		}
		return netConn, err
	}
}

func (RedisHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, command redis.Cmder) error {
		ctx, span := otel.Tracer("ecommerce-core/redis").Start(ctx, "redis "+command.Name(), trace.WithSpanKind(trace.SpanKindClient), trace.WithAttributes(attribute.String("db.operation.name", command.Name())))
		defer span.End()
		err := next(ctx, command)
		if err != nil {
			span.RecordError(err)
		}
		return err
	}
}

func (RedisHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, commands []redis.Cmder) error {
		ctx, span := otel.Tracer("ecommerce-core/redis").Start(ctx, "redis pipeline", trace.WithSpanKind(trace.SpanKindClient), trace.WithAttributes(attribute.Int("db.redis.pipeline_length", len(commands))))
		defer span.End()
		err := next(ctx, commands)
		if err != nil {
			span.RecordError(err)
		}
		return err
	}
}
