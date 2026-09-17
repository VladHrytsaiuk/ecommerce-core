// Package cache implements provider-neutral cache ports using Redis.
package cache

import (
	"context"
	"fmt"
	"time"

	redisgo "github.com/redis/go-redis/v9"

	shared "github.com/VladHrytsaiuk/ecommerce-core/internal/shared/cache"
)

type RedisAdapter struct {
	client *redisgo.Client
	prefix string
}

func NewRedisAdapter(client *redisgo.Client, prefix string) *RedisAdapter {
	return &RedisAdapter{client: client, prefix: prefix}
}

func (a *RedisAdapter) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if err := a.client.Set(ctx, a.key(key), value, ttl).Err(); err != nil {
		return fmt.Errorf("cache set: %w", err)
	}
	return nil
}

func (a *RedisAdapter) Get(ctx context.Context, key string) ([]byte, error) {
	value, err := a.client.Get(ctx, a.key(key)).Bytes()
	if err == redisgo.Nil {
		return nil, shared.ErrMiss
	}
	if err != nil {
		return nil, fmt.Errorf("cache get: %w", err)
	}
	return value, nil
}

func (a *RedisAdapter) Delete(ctx context.Context, key string) error {
	return a.client.Del(ctx, a.key(key)).Err()
}

// DeleteByPrefix uses SCAN rather than KEYS, preventing a blocking full-keyspace
// query in production.
func (a *RedisAdapter) DeleteByPrefix(ctx context.Context, prefix string) error {
	pattern := a.key(prefix) + "*"
	var cursor uint64
	for {
		keys, next, err := a.client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return fmt.Errorf("cache scan: %w", err)
		}
		if len(keys) > 0 {
			if err := a.client.Del(ctx, keys...).Err(); err != nil {
				return fmt.Errorf("cache delete prefix: %w", err)
			}
		}
		cursor = next
		if cursor == 0 {
			return nil
		}
	}
}

func (a *RedisAdapter) key(key string) string { return a.prefix + key }
