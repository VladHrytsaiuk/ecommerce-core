package application

import (
	"context"
	"time"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/worker"
)

type ExpiredReservationStore interface {
	ReleaseExpiredUnattached(context.Context, time.Time, int) (int, error)
}

type Cleanup struct {
	store  ExpiredReservationStore
	logger worker.Logger
}

func NewCleanup(store ExpiredReservationStore) *Cleanup { return &Cleanup{store: store} }

// WithLogger makes a failing sweep visible. Without it the worker is silent by
// construction: expired reservations go on holding stock that the catalog
// still reports as reserved, and nothing anywhere says why.
func (c *Cleanup) WithLogger(logger worker.Logger) *Cleanup {
	if c != nil && logger != nil {
		c.logger = logger
	}
	return c
}

// releaseBatch is one statement's worth of reservations. The loop drains, so
// this bounds a single pass through the store rather than the work per tick.
const releaseBatch = 100

// ReleaseOnce settles one batch and reports whether it filled it, which is the
// only signal that more may be waiting.
func (c *Cleanup) ReleaseOnce(ctx context.Context) (bool, error) {
	released, err := c.store.ReleaseExpiredUnattached(ctx, time.Now().UTC(), releaseBatch)
	if err != nil {
		return false, err
	}
	return released == releaseBatch, nil
}

// Run drains the expired backlog each tick.
//
// It used to take a single page of a hundred per minute and explicitly not
// drain, on the reasoning that this is a reconciliation rather than a queue.
// That holds only while expiries arrive slower than the sweep clears them:
// above a hundred abandoned checkouts a minute — a flash sale, a payment
// provider outage — the sweep fell permanently behind, and stock stayed
// reserved for orders that no longer existed while the catalog reported it
// unavailable. Every other worker in this core drains for the same reason.
func (c *Cleanup) Run(ctx context.Context, interval time.Duration) {
	worker.LoopDraining(ctx, interval, time.Minute, c.logger, "inventory expired reservation sweep", c.ReleaseOnce)
}
