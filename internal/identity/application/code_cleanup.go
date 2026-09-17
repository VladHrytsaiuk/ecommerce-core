package application

import (
	"context"
	"fmt"
	"time"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/worker"
)

// SettledCodeStore removes one-time codes that no longer matter.
type SettledCodeStore interface {
	PurgeSettled(context.Context, time.Time, int) (int, error)
}

const codePurgeBatch = 500

// CodeCleanup removes one-time codes once they can neither be used nor
// count towards an address's sending limits. Every request for a code adds a
// row.
type CodeCleanup struct {
	store  SettledCodeStore
	logger worker.Logger
	now    func() time.Time
}

func NewCodeCleanup(store SettledCodeStore) (*CodeCleanup, error) {
	if store == nil {
		return nil, fmt.Errorf("one-time code cleanup requires a store")
	}
	return &CodeCleanup{store: store, now: func() time.Time { return time.Now().UTC() }}, nil
}

// WithLogger makes a failing sweep visible.
func (c *CodeCleanup) WithLogger(logger worker.Logger) *CodeCleanup {
	if c != nil && logger != nil {
		c.logger = logger
	}
	return c
}

// PurgeOnce removes one batch and reports whether it filled it.
func (c *CodeCleanup) PurgeOnce(ctx context.Context) (bool, error) {
	removed, err := c.store.PurgeSettled(ctx, c.now(), codePurgeBatch)
	if err != nil {
		return false, err
	}
	return removed == codePurgeBatch, nil
}

// Run drains the settled backlog each tick.
func (c *CodeCleanup) Run(ctx context.Context, interval time.Duration) {
	worker.LoopDraining(ctx, interval, time.Hour, c.logger, "one-time code sweep", c.PurgeOnce)
}
