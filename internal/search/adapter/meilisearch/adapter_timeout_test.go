package meilisearch

import (
	"context"
	"testing"
	"time"
)

func TestTaskContextHasBoundedDeadline(t *testing.T) {
	adapter := &Adapter{taskTimeout: 20 * time.Millisecond}
	ctx, cancel := adapter.taskContext(context.Background())
	defer cancel()
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) <= 0 || time.Until(deadline) > 30*time.Millisecond {
		t.Fatalf("task context deadline = %v, present=%t", deadline, ok)
	}
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("task context did not enforce its deadline")
	}
}

func TestTaskContextPreservesEarlierParentDeadline(t *testing.T) {
	parent, parentCancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer parentCancel()
	ctx, cancel := (&Adapter{taskTimeout: time.Second}).taskContext(parent)
	defer cancel()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("task context ignored earlier parent deadline")
	}
}
