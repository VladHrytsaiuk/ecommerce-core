// Package redis owns Redis connectivity. Feature modules receive only ports.
package redis

import (
	"context"
	"errors"
	"fmt"
	"time"

	redisgo "github.com/redis/go-redis/v9"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/observability"
)

const connectTimeout = 3 * time.Second

// ErrInvalidURL intentionally has no parsing detail: net/url errors may echo
// the complete input URL, whose userinfo can contain a Redis password.
var ErrInvalidURL = errors.New("invalid REDIS_URL")

type Client struct{ raw *redisgo.Client }

func Connect(ctx context.Context, rawURL string) (*Client, error) {
	options, err := redisgo.ParseURL(rawURL)
	if err != nil {
		return nil, ErrInvalidURL
	}
	client := redisgo.NewClient(options)
	client.AddHook(observability.NewRedisHook())
	pingCtx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()
	if err := client.Ping(pingCtx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("ping Redis: %w", err)
	}
	return &Client{raw: client}, nil
}

func (c *Client) Raw() *redisgo.Client { return c.raw }
func (c *Client) Close() error         { return c.raw.Close() }
