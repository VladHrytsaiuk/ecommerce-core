package application

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type sweepStore struct {
	mu    sync.Mutex
	calls int
	err   error
	// released is what a sweep reports, and releaseUntil is how many sweeps
	// report it before the store runs dry. Together they describe a backlog
	// deeper than one batch.
	released     int
	releaseUntil int
}

func (s *sweepStore) ReleaseExpiredUnattached(context.Context, time.Time, int) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if s.err != nil {
		return 0, s.err
	}
	if s.calls <= s.releaseUntil {
		return s.released, nil
	}
	return 0, nil
}

func (s *sweepStore) sweeps() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

type recordingLogger struct {
	mu       sync.Mutex
	messages []string
}

func (l *recordingLogger) Errorw(message string, _ ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.messages = append(l.messages, message)
}

func (l *recordingLogger) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.messages)
}

func TestCleanupReportsAFailedSweep(t *testing.T) {
	// A failing sweep used to be indistinguishable from an idle one: the
	// worker discarded both return values, so expired reservations went on
	// holding stock with nothing anywhere saying why.
	store := &sweepStore{err: errors.New("connection reset by peer")}
	logger := &recordingLogger{}
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		NewCleanup(store).WithLogger(logger).Run(ctx, time.Hour)
		close(done)
	}()
	waitFor(t, func() bool { return logger.count() > 0 })
	cancel()
	<-done

	if logger.count() == 0 {
		t.Fatal("a failing sweep produced no log entry")
	}
}

func TestCleanupDoesNotReportItsOwnShutdown(t *testing.T) {
	// Shutdown cancels the context mid-sweep. Logging that as an error trains
	// operators to ignore the message that matters.
	store := &sweepStore{err: context.Canceled}
	logger := &recordingLogger{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	NewCleanup(store).WithLogger(logger).Run(ctx, time.Hour)

	if logger.count() != 0 {
		t.Fatalf("shutdown logged %d errors, want 0", logger.count())
	}
}

func TestCleanupWithoutALoggerStillSweeps(t *testing.T) {
	// Bootstrap supplies a logger, but the zero value must not panic: tests
	// and tooling construct this worker directly.
	store := &sweepStore{err: errors.New("connection reset by peer")}
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		NewCleanup(store).Run(ctx, time.Hour)
		close(done)
	}()
	waitFor(t, func() bool { return store.sweeps() > 0 })
	cancel()
	<-done
}

func TestCleanupDrainsTheBacklogWithinOneTick(t *testing.T) {
	// It used to take one page of a hundred per minute and stop. Above that
	// rate — a flash sale, a payment outage — the sweep fell permanently
	// behind and stock stayed reserved for orders that no longer existed.
	//
	// The interval is an hour, so anything past the first sweep here happened
	// inside a single tick.
	store := &sweepStore{released: releaseBatch, releaseUntil: 3}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		NewCleanup(store).Run(ctx, time.Hour)
		close(done)
	}()
	waitFor(t, func() bool { return store.sweeps() >= 4 })
	cancel()
	<-done

	// Three full batches, then a short one that ends the drain.
	if swept := store.sweeps(); swept != 4 {
		t.Fatalf("sweeps = %d, want the drain to stop at the first short batch", swept)
	}
}

func TestCleanupStopsDrainingWhenABatchFails(t *testing.T) {
	// A failing store must end the pass, not spin through the ceiling against
	// a database it cannot reach.
	store := &sweepStore{released: releaseBatch, err: errors.New("connection reset by peer")}
	logger := &recordingLogger{}
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		NewCleanup(store).WithLogger(logger).Run(ctx, time.Hour)
		close(done)
	}()
	waitFor(t, func() bool { return logger.count() > 0 })
	cancel()
	<-done

	if swept := store.sweeps(); swept != 1 {
		t.Fatalf("sweeps = %d, want the pass to end on the first failure", swept)
	}
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition not met within the deadline")
}
