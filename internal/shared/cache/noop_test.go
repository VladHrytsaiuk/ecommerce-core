package cache

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestNoOpServiceIsTransparent(t *testing.T) {
	service := NewNoOpService()
	if err := service.Set(context.Background(), "key", []byte("value"), time.Minute); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if _, err := service.Get(context.Background(), "key"); !errors.Is(err, ErrMiss) {
		t.Fatalf("Get() error = %v, want ErrMiss", err)
	}
	if err := service.DeleteByPrefix(context.Background(), "key"); err != nil {
		t.Fatalf("DeleteByPrefix() error = %v", err)
	}
}
