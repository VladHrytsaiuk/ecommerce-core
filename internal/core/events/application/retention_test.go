package application

import (
	"context"
	"testing"
	"time"
)

func TestRetentionWorkerArchivesTerminalDeliveriesUsingConfiguredCutoff(t *testing.T) {
	store := &fakeRetentionStore{}
	worker, err := NewRetentionWorker(store, 24*time.Hour, 100, nil)
	if err != nil {
		t.Fatalf("NewRetentionWorker() error = %v", err)
	}
	before := time.Now().UTC().Add(-24 * time.Hour)
	if count, err := worker.ArchiveOnce(context.Background()); err != nil || count != 3 {
		t.Fatalf("ArchiveOnce() = %d, %v", count, err)
	}
	if store.limit != 100 || store.cutoff.Before(before.Add(-time.Second)) || store.cutoff.After(time.Now().UTC().Add(-24*time.Hour+time.Second)) {
		t.Fatalf("archive request = cutoff %s, limit %d", store.cutoff, store.limit)
	}
}

type fakeRetentionStore struct {
	cutoff time.Time
	limit  int
}

func (s *fakeRetentionStore) ArchiveDone(_ context.Context, cutoff time.Time, limit int) (int, error) {
	s.cutoff, s.limit = cutoff, limit
	return 3, nil
}
