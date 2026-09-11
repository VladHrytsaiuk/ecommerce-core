// Package worker holds the loop every background worker in this core runs.
//
// It exists because that loop was copied into eleven places and the copies
// drifted. Some drained a backlog per tick and some took one item, so a
// dispatcher on a five-second interval settled twelve jobs an hour regardless
// of how many were waiting. Most discarded the error their step returned, so a
// worker that could not reach its database looked exactly like one with
// nothing to do.
package worker

import (
	"context"
	"time"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/sanitize"
)

// DefaultDrainCeiling bounds one drain pass. A worker with a large backlog
// must not hold its goroutine indefinitely or starve the shutdown path, and a
// ceiling keeps each pass's duration bounded and predictable.
const DefaultDrainCeiling = 256

// Logger is the narrow reporting contract. It is satisfied by the platform
// logger and is deliberately not required: a worker constructed without one
// still runs, it just says nothing.
type Logger interface {
	Errorw(string, ...interface{})
}

// Step performs one unit of work and reports whether it found any. Returning
// false ends the drain: the queue is empty and further claims would be wasted
// round trips.
type Step func(context.Context) (worked bool, err error)

// Drain runs step until it finds no work, fails, the context ends, or the
// ceiling is reached.
func Drain(ctx context.Context, ceiling int, step Step) error {
	if ceiling <= 0 {
		ceiling = DefaultDrainCeiling
	}
	for range ceiling {
		if ctx.Err() != nil {
			return nil
		}
		worked, err := step(ctx)
		if err != nil {
			return err
		}
		if !worked {
			return nil
		}
	}
	return nil
}

// Loop runs pass on a ticker until the context ends, reporting each failure
// under name.
//
// A cancelled context is shutdown, not breakage: reporting it as an error
// teaches operators to ignore the message that matters, so it is dropped.
func Loop(ctx context.Context, interval, fallback time.Duration, logger Logger, name string, pass func(context.Context) error) {
	if interval <= 0 {
		interval = fallback
	}
	if interval <= 0 {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := pass(ctx); err != nil && ctx.Err() == nil && logger != nil {
			logger.Errorw(name+" failed", "error_code", sanitize.ErrorCode(err))
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// LoopDraining is the shape most workers want: drain a backlog each tick and
// report what went wrong.
func LoopDraining(ctx context.Context, interval, fallback time.Duration, logger Logger, name string, step Step) {
	Loop(ctx, interval, fallback, logger, name, func(ctx context.Context) error {
		return Drain(ctx, DefaultDrainCeiling, step)
	})
}
