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

// Run sweeps a page of expired reservations each tick. It does not drain: the
// sweep is a periodic reconciliation against wall-clock expiry, not a queue,
// and a pass that fails part way still released what it released.
func (c *Cleanup) Run(ctx context.Context, interval time.Duration) {
	worker.Loop(ctx, interval, time.Minute, c.logger, "inventory expired reservation sweep", func(ctx context.Context) error {
		_, err := c.store.ReleaseExpiredUnattached(ctx, time.Now().UTC(), 100)
		return err
	})
}
