package application

import (
	"context"
	"testing"
	"time"
)

func TestRetentionWorkerArchivesTerminalDeliveriesUsingConfiguredCutoff(t *testing.T) {
	store := &fakeRetentionStore{}
	worker, err := NewRetentionWorker(store, 24*time.Hour, 72*time.Hour, 100, nil)
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

func TestRetentionWorkerPrunesTheArchiveOnItsOwnLongerCutoff(t *testing.T) {
	// Archiving alone moved rows between two tables and nothing emptied the
	// second, so the database grew as fast as with no retention at all. The
	// prune uses the archive window, not the done window — mixing them up
	// would delete a delivery the moment it was archived.
	store := &fakeRetentionStore{}
	worker, err := NewRetentionWorker(store, 24*time.Hour, 72*time.Hour, 100, nil)
	if err != nil {
		t.Fatalf("NewRetentionWorker() error = %v", err)
	}
	expected := time.Now().UTC().Add(-72 * time.Hour)
	if count, err := worker.PruneOnce(context.Background()); err != nil || count != 2 {
		t.Fatalf("PruneOnce() = %d, %v", count, err)
	}
	if store.pruneLimit != 100 {
		t.Fatalf("prune limit = %d, want the configured batch size", store.pruneLimit)
	}
	if store.pruneCutoff.Before(expected.Add(-time.Second)) || store.pruneCutoff.After(expected.Add(time.Second)) {
		t.Fatalf("prune cutoff = %s, want about %s", store.pruneCutoff, expected)
	}
}

func TestRetentionWorkerRefusesAnArchiveWindowShorterThanTheDoneWindow(t *testing.T) {
	// A delivery only reaches the archive after the done window has elapsed.
	// A shorter archive window deletes it on arrival, leaving no history and
	// no error to say so.
	if _, err := NewRetentionWorker(&fakeRetentionStore{}, 72*time.Hour, 24*time.Hour, 100, nil); err == nil {
		t.Fatal("NewRetentionWorker() accepted an archive window that deletes on arrival")
	}
}

type fakeRetentionStore struct {
	cutoff      time.Time
	limit       int
	pruneCutoff time.Time
	pruneLimit  int
}

func (s *fakeRetentionStore) ArchiveDone(_ context.Context, cutoff time.Time, limit int) (int, error) {
	s.cutoff, s.limit = cutoff, limit
	return 3, nil
}

func (s *fakeRetentionStore) PruneArchive(_ context.Context, cutoff time.Time, limit int) (int, error) {
	s.pruneCutoff, s.pruneLimit = cutoff, limit
	return 2, nil
}
