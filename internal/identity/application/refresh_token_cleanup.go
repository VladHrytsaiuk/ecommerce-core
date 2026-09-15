package application

import (
	"context"
	"fmt"
	"time"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/worker"
)

// ExpiredRefreshTokenStore removes refresh tokens whose sign-in has ended.
type ExpiredRefreshTokenStore interface {
	PurgeExpired(context.Context, time.Time, int) (int, error)
}

const refreshTokenPurgeBatch = 500

// RefreshTokenCleanup removes refresh tokens once their sign-in has ended.
// Every refresh adds a row, so without it the table grows with every customer's
// every fifteen minutes of activity.
type RefreshTokenCleanup struct {
	store  ExpiredRefreshTokenStore
	logger worker.Logger
	now    func() time.Time
}

func NewRefreshTokenCleanup(store ExpiredRefreshTokenStore) (*RefreshTokenCleanup, error) {
	if store == nil {
		return nil, fmt.Errorf("refresh token cleanup requires a store")
	}
	return &RefreshTokenCleanup{store: store, now: func() time.Time { return time.Now().UTC() }}, nil
}

// WithLogger makes a failing sweep visible.
func (c *RefreshTokenCleanup) WithLogger(logger worker.Logger) *RefreshTokenCleanup {
	if c != nil && logger != nil {
		c.logger = logger
	}
	return c
}

// PurgeOnce removes one batch and reports whether it filled it.
func (c *RefreshTokenCleanup) PurgeOnce(ctx context.Context) (bool, error) {
	removed, err := c.store.PurgeExpired(ctx, c.now(), refreshTokenPurgeBatch)
	if err != nil {
		return false, err
	}
	return removed == refreshTokenPurgeBatch, nil
}

// Run drains the expired backlog each tick.
func (c *RefreshTokenCleanup) Run(ctx context.Context, interval time.Duration) {
	worker.LoopDraining(ctx, interval, time.Hour, c.logger, "refresh token sweep", c.PurgeOnce)
}
