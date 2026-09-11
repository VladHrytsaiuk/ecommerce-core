package worker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestDrainKeepsGoingUntilTheQueueIsEmpty(t *testing.T) {
	// The behaviour the one-item-per-tick workers lacked: a backlog is cleared
	// in one pass rather than at the rate of the ticker.
	remaining := 5
	calls := 0

	if err := Drain(context.Background(), DefaultDrainCeiling, func(context.Context) (bool, error) {
		calls++
		if remaining == 0 {
			return false, nil
		}
		remaining--
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatalf("remaining = %d, want the backlog drained in one pass", remaining)
	}
	// One call past the last item is how the drain learns the queue is empty.
	if calls != 6 {
		t.Fatalf("calls = %d, want 6", calls)
	}
}

func TestDrainStopsAtTheCeiling(t *testing.T) {
	// An unbounded drain would hold the goroutine and starve shutdown.
	calls := 0

	if err := Drain(context.Background(), 3, func(context.Context) (bool, error) {
		calls++
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	if calls != 3 {
		t.Fatalf("calls = %d, want the pass bounded at 3", calls)
	}
}

func TestDrainStopsOnFailureAndReportsIt(t *testing.T) {
	failure := errors.New("store unavailable")
	calls := 0

	err := Drain(context.Background(), DefaultDrainCeiling, func(context.Context) (bool, error) {
		calls++
		return true, failure
	})
	if !errors.Is(err, failure) {
		t.Fatalf("Drain() error = %v, want the step's failure", err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want the drain to stop on the first failure", calls)
	}
}

func TestDrainStopsWhenTheContextEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0

	if err := Drain(ctx, DefaultDrainCeiling, func(context.Context) (bool, error) {
		calls++
		return true, nil
	}); err != nil {
		t.Fatalf("Drain() error = %v, want a cancelled context to end quietly", err)
	}
	if calls != 0 {
		t.Fatalf("calls = %d, want none after cancellation", calls)
	}
}

func TestLoopReportsAFailingPass(t *testing.T) {
	// Discarding this error is what made a worker that could not reach its
	// database indistinguishable from one with nothing to do.
	logger := &recordingLogger{}
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		Loop(ctx, time.Hour, time.Hour, logger, "test worker", func(context.Context) error {
			return errors.New("store unavailable")
		})
		close(done)
	}()
	waitFor(t, func() bool { return logger.count() > 0 })
	cancel()
	<-done

	if got := logger.message(0); got != "test worker failed" {
		t.Fatalf("logged %q, want the worker named", got)
	}
}

func TestLoopDoesNotReportItsOwnShutdown(t *testing.T) {
	logger := &recordingLogger{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	Loop(ctx, time.Hour, time.Hour, logger, "test worker", func(ctx context.Context) error {
		return ctx.Err()
	})

	if logger.count() != 0 {
		t.Fatalf("shutdown logged %d errors, want 0", logger.count())
	}
}

func TestLoopRunsWithoutALogger(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	passes := 0

	Loop(ctx, 0, 0, nil, "test worker", func(context.Context) error {
		passes++
		return errors.New("store unavailable")
	})

	if passes != 1 {
		t.Fatalf("passes = %d, want the pass to run and the failure to be dropped quietly", passes)
	}
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

func (l *recordingLogger) message(index int) string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.messages[index]
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
