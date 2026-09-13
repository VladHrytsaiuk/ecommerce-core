package application

import (
	"context"
	"fmt"
	"time"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/worker"
)

// RetentionStore purges terminal notification jobs and reports how many
// messages never went out.
type RetentionStore interface {
	PurgeTerminal(ctx context.Context, sentBefore, deadBefore time.Time, limit int) (int, error)
	CountDead(ctx context.Context) (int, error)
}

// DeadJobRecorder publishes the number of messages that never went out.
//
// Retention deletes dead jobs eventually, so without this the cleanup would
// quietly erase the evidence that a store has been failing to send mail: the
// table empties and nothing ever said why. Platform implements it with a gauge;
// a deployment without metrics uses the no-op default.
type DeadJobRecorder interface {
	DeadNotificationJobs(count int)
}

type noopDeadJobRecorder struct{}

func (noopDeadJobRecorder) DeadNotificationJobs(int) {}

// RetentionWorker bounds how long notification jobs are kept.
//
// The table holds one row per message ever sent, including the recipient's
// address and, on the scheduled path, the rendered body. It had no window at
// all and was not in the retention decisions recorded in migrations/README.md,
// so it grew for the life of the store.
type RetentionWorker struct {
	store         RetentionStore
	sentRetention time.Duration
	deadRetention time.Duration
	batchSize     int
	recorder      DeadJobRecorder
	logger        worker.Logger
}

func NewRetentionWorker(store RetentionStore, sentRetention, deadRetention time.Duration, batchSize int, logger worker.Logger) (*RetentionWorker, error) {
	if store == nil || sentRetention <= 0 || deadRetention <= 0 || batchSize < 1 || batchSize > 10000 {
		return nil, fmt.Errorf("invalid notification retention worker configuration")
	}
	return &RetentionWorker{
		store: store, sentRetention: sentRetention, deadRetention: deadRetention,
		batchSize: batchSize, recorder: noopDeadJobRecorder{}, logger: logger,
	}, nil
}

// WithMetrics injects the platform gauge at the composition root.
func (w *RetentionWorker) WithMetrics(recorder DeadJobRecorder) *RetentionWorker {
	if recorder != nil {
		w.recorder = recorder
	}
	return w
}

// PurgeCycle is one tick: report the backlog, then drain it.
//
// The report comes first so the number an operator sees is never one this cycle
// has already reduced, and it comes once — not once per batch. It used to sit
// inside the batch, which put a full count between every delete: the drain runs
// up to 256 times a tick, so a single cycle scanned the table hundreds of times
// for a gauge that changes once.
func (w *RetentionWorker) PurgeCycle(ctx context.Context) error {
	w.reportDeadBacklog(ctx)
	return worker.Drain(ctx, worker.DefaultDrainCeiling, func(ctx context.Context) (bool, error) {
		purged, err := w.PurgeOnce(ctx)
		return purged > 0, err
	})
}

func (w *RetentionWorker) reportDeadBacklog(ctx context.Context) {
	dead, err := w.store.CountDead(ctx)
	if err != nil {
		if w.logger != nil {
			w.logger.Errorw("counting dead notification jobs failed", "error", err)
		}
		return
	}
	w.recorder.DeadNotificationJobs(dead)
}

// PurgeOnce deletes one bounded batch.
func (w *RetentionWorker) PurgeOnce(ctx context.Context) (int, error) {
	now := time.Now().UTC()
	return w.store.PurgeTerminal(ctx, now.Add(-w.sentRetention), now.Add(-w.deadRetention), w.batchSize)
}

// Run drains the backlog rather than deleting one batch per tick, so a table
// that has grown for months converges instead of shrinking by one batch an hour.
func (w *RetentionWorker) Run(ctx context.Context, interval time.Duration) {
	worker.Loop(ctx, interval, time.Hour, w.logger, "notification retention", w.PurgeCycle)
}
