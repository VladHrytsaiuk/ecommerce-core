package redis

import (
	"context"
	"fmt"
	"time"

	redisgo "github.com/redis/go-redis/v9"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/ratelimit"
)

var fixedWindowScript = redisgo.NewScript(`
local count = redis.call("INCR", KEYS[1])
if count == 1 then
  redis.call("EXPIRE", KEYS[1], ARGV[1])
end
local ttl = redis.call("TTL", KEYS[1])
return {count, ttl}
`)

type FixedWindowLimiter struct{ client *redisgo.Client }

func NewFixedWindowLimiter(client *redisgo.Client) *FixedWindowLimiter {
	return &FixedWindowLimiter{client: client}
}

func (l *FixedWindowLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (ratelimit.Decision, error) {
	result, err := fixedWindowScript.Run(ctx, l.client, []string{key}, int64(window.Seconds())).Int64Slice()
	if err != nil || len(result) != 2 {
		if err == nil {
			err = fmt.Errorf("unexpected Redis rate-limit response")
		}
		return ratelimit.Decision{}, fmt.Errorf("redis rate limit: %w", err)
	}
	retryAfter := time.Duration(result[1]) * time.Second
	if retryAfter < time.Second {
		retryAfter = time.Second
	}
	return ratelimit.Decision{Allowed: result[0] <= int64(limit), RetryAfter: retryAfter}, nil
}
