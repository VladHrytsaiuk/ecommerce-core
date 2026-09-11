package application

import (
	"context"
	"errors"
	"time"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/sanitize"
)

type ExpiredReservationStore interface {
	ReleaseExpiredUnattached(context.Context, time.Time, int) (int, error)
}

// Logger is the narrow contract this worker needs. It mirrors the outbox
// worker's so Bootstrap can pass the same logger to both.
type Logger interface {
	Errorw(string, ...interface{})
}

type Cleanup struct {
	store  ExpiredReservationStore
	logger Logger
}

func NewCleanup(store ExpiredReservationStore) *Cleanup { return &Cleanup{store: store} }

// WithLogger makes a failing sweep visible. Without it the worker is silent by
// construction: expired reservations go on holding stock that the catalog
// still reports as reserved, and nothing anywhere says why.
func (c *Cleanup) WithLogger(logger Logger) *Cleanup {
	if c != nil && logger != nil {
		c.logger = logger
	}
	return c
}

func (c *Cleanup) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		// A sweep that fails part way still released what it released; the
		// next tick picks up the rest. Only the reason is worth reporting.
		if _, err := c.store.ReleaseExpiredUnattached(ctx, time.Now().UTC(), 100); err != nil {
			c.report(ctx, err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (c *Cleanup) report(ctx context.Context, err error) {
	// Shutdown cancels the context mid-sweep. That is the worker stopping, not
	// the sweep breaking, and logging it as an error trains operators to
	// ignore the message that matters.
	if errors.Is(err, context.Canceled) || ctx.Err() != nil {
		return
	}
	if c.logger == nil {
		return
	}
	c.logger.Errorw("inventory expired reservation sweep failed", "error_code", sanitize.ErrorCode(err))
}
