package application

import (
	"context"
	"fmt"
	"time"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/worker"
)

// SettledSignInCodeStore removes sign-in codes that no longer matter.
type SettledSignInCodeStore interface {
	PurgeSettled(context.Context, time.Time, int) (int, error)
}

const signInCodePurgeBatch = 500

// SignInCodeCleanup removes sign-in codes once they can neither be used nor
// count towards an address's sending limits. Every request for a code adds a
// row.
type SignInCodeCleanup struct {
	store  SettledSignInCodeStore
	logger worker.Logger
	now    func() time.Time
}

func NewSignInCodeCleanup(store SettledSignInCodeStore) (*SignInCodeCleanup, error) {
	if store == nil {
		return nil, fmt.Errorf("sign-in code cleanup requires a store")
	}
	return &SignInCodeCleanup{store: store, now: func() time.Time { return time.Now().UTC() }}, nil
}

// WithLogger makes a failing sweep visible.
func (c *SignInCodeCleanup) WithLogger(logger worker.Logger) *SignInCodeCleanup {
	if c != nil && logger != nil {
		c.logger = logger
	}
	return c
}

// PurgeOnce removes one batch and reports whether it filled it.
func (c *SignInCodeCleanup) PurgeOnce(ctx context.Context) (bool, error) {
	removed, err := c.store.PurgeSettled(ctx, c.now(), signInCodePurgeBatch)
	if err != nil {
		return false, err
	}
	return removed == signInCodePurgeBatch, nil
}

// Run drains the settled backlog each tick.
func (c *SignInCodeCleanup) Run(ctx context.Context, interval time.Duration) {
	worker.LoopDraining(ctx, interval, time.Hour, c.logger, "sign-in code sweep", c.PurgeOnce)
}
