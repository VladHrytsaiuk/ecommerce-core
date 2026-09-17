package application

import (
	"context"
	"fmt"
	"time"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/worker"
)

// SettledAttemptStore removes OAuth authorization attempts that can no longer
// be consumed, in bounded batches.
type SettledAttemptStore interface {
	PurgeSettled(context.Context, time.Time, int) (int, error)
}

// attemptPurgeBatch is one DELETE's worth. The loop drains, so the batch size
// bounds a single statement rather than the work done per tick.
const attemptPurgeBatch = 500

// OAuthAttemptCleanup removes authorization attempts once they are consumed or
// expired.
//
// The table is ephemeral by contract — an attempt is valid for OAUTH_ATTEMPT_TTL
// and useless after — but nothing ever deleted a row, so it grew by one per
// sign-in click forever, each row holding that exchange's PKCE verifier and
// nonce. Every other short-lived table in this core already has a sweep;
// this one did not.
type OAuthAttemptCleanup struct {
	store  SettledAttemptStore
	logger worker.Logger
	now    func() time.Time
}

func NewOAuthAttemptCleanup(store SettledAttemptStore) (*OAuthAttemptCleanup, error) {
	if store == nil {
		return nil, fmt.Errorf("OAuth attempt cleanup requires a store")
	}
	return &OAuthAttemptCleanup{store: store, now: func() time.Time { return time.Now().UTC() }}, nil
}

// WithLogger makes a failing sweep visible. Without it the worker is silent by
// construction, and a table that cannot be pruned looks exactly like one with
// nothing to prune.
func (c *OAuthAttemptCleanup) WithLogger(logger worker.Logger) *OAuthAttemptCleanup {
	if c != nil && logger != nil {
		c.logger = logger
	}
	return c
}

// PurgeOnce removes one batch and reports whether it found any. It is the step
// Run drains, and the entry point for a test that wants a single pass.
func (c *OAuthAttemptCleanup) PurgeOnce(ctx context.Context) (bool, error) {
	removed, err := c.store.PurgeSettled(ctx, c.now(), attemptPurgeBatch)
	if err != nil {
		return false, err
	}
	return removed == attemptPurgeBatch, nil
}

// Run drains the backlog each tick rather than taking a single batch: a burst
// of sign-ins must not leave rows the sweep can never catch up with.
func (c *OAuthAttemptCleanup) Run(ctx context.Context, interval time.Duration) {
	worker.LoopDraining(ctx, interval, time.Hour, c.logger, "OAuth authorization attempt sweep", c.PurgeOnce)
}
