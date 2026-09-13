package application

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Retention deletes the record that a message was never delivered, so the count
// has to be published before anything is removed and the windows have to be
// right. Getting either wrong erases evidence rather than old rows.

func TestTheDeadBacklogIsReportedBeforeAnythingIsDeleted(t *testing.T) {
	store := &retentionStoreStub{dead: 7}
	recorder := &deadJobRecorderStub{}
	worker := mustRetentionWorker(t, store, 30*24*time.Hour, 90*24*time.Hour).WithMetrics(recorder)

	if _, err := worker.PurgeOnce(context.Background()); err != nil {
		t.Fatalf("PurgeOnce() error = %v", err)
	}
	if recorder.reported != 7 {
		t.Fatalf("reported %d dead jobs, want 7", recorder.reported)
	}
	if !store.countedBeforePurge {
		t.Fatal("the backlog was counted after the purge, so the number hides what this pass removed")
	}
}

func TestTheTwoWindowsAreAppliedSeparately(t *testing.T) {
	// A sent job is evidence a message went out. A dead one is a message that
	// never did, which an operator may still need, so it is kept far longer.
	store := &retentionStoreStub{}
	worker := mustRetentionWorker(t, store, 30*24*time.Hour, 90*24*time.Hour)
	before := time.Now().UTC()

	if _, err := worker.PurgeOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	// A minute of tolerance: the worker takes its own clock reading a moment
	// after this one, and the point is which window applies, not the instant.
	const tolerance = time.Minute
	if age := before.Sub(store.sentBefore); age < 30*24*time.Hour-tolerance || age > 30*24*time.Hour+tolerance {
		t.Fatalf("sent cutoff is %s old, want 30 days", age)
	}
	if age := before.Sub(store.deadBefore); age < 90*24*time.Hour-tolerance || age > 90*24*time.Hour+tolerance {
		t.Fatalf("dead cutoff is %s old, want 90 days", age)
	}
}

func TestDeletionIsBounded(t *testing.T) {
	// An unbounded delete on a table that has grown for months locks it for
	// the length of the statement.
	store := &retentionStoreStub{}
	if _, err := mustRetentionWorker(t, store, time.Hour, time.Hour).PurgeOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.limit < 1 || store.limit > 10000 {
		t.Fatalf("batch size = %d, want a bounded batch", store.limit)
	}
}

func TestAFailingCountDoesNotStopThePurge(t *testing.T) {
	// The gauge is reporting; the purge is the job. One must not block the other.
	store := &retentionStoreStub{countErr: errors.New("database unavailable")}
	worker := mustRetentionWorker(t, store, time.Hour, time.Hour)

	if _, err := worker.PurgeOnce(context.Background()); err != nil {
		t.Fatalf("PurgeOnce() error = %v", err)
	}
	if !store.purged {
		t.Fatal("a failed count stopped the purge")
	}
}

func TestAWorkerWithoutMetricsStillPurges(t *testing.T) {
	store := &retentionStoreStub{dead: 3}
	if _, err := mustRetentionWorker(t, store, time.Hour, time.Hour).WithMetrics(nil).PurgeOnce(context.Background()); err != nil {
		t.Fatalf("PurgeOnce() error = %v", err)
	}
	if !store.purged {
		t.Fatal("no purge without a recorder")
	}
}

func TestARetentionWorkerIsRefusedWithoutWindows(t *testing.T) {
	for name, scenario := range map[string]struct{ sent, dead time.Duration }{
		"no sent window": {0, time.Hour},
		"no dead window": {time.Hour, 0},
		"negative":       {-time.Hour, time.Hour},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NewRetentionWorker(&retentionStoreStub{}, scenario.sent, scenario.dead, 100, nil); err == nil {
				t.Fatal("NewRetentionWorker() accepted a window that deletes everything")
			}
		})
	}
}

func mustRetentionWorker(t *testing.T, store RetentionStore, sent, dead time.Duration) *RetentionWorker {
	t.Helper()
	worker, err := NewRetentionWorker(store, sent, dead, 1000, nil)
	if err != nil {
		t.Fatal(err)
	}
	return worker
}

type retentionStoreStub struct {
	dead                   int
	countErr               error
	sentBefore, deadBefore time.Time
	limit                  int
	purged                 bool
	countedBeforePurge     bool
	counted                bool
}

func (s *retentionStoreStub) CountDead(context.Context) (int, error) {
	if s.countErr != nil {
		return 0, s.countErr
	}
	s.counted = true
	return s.dead, nil
}

func (s *retentionStoreStub) PurgeTerminal(_ context.Context, sentBefore, deadBefore time.Time, limit int) (int, error) {
	s.countedBeforePurge = s.counted
	s.sentBefore, s.deadBefore, s.limit, s.purged = sentBefore, deadBefore, limit, true
	return 0, nil
}

type deadJobRecorderStub struct{ reported int }

func (r *deadJobRecorderStub) DeadNotificationJobs(count int) { r.reported = count }
