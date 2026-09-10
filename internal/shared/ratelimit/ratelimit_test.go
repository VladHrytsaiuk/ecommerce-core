package ratelimit

import (
	"context"
	"strconv"
	"testing"
	"time"
)

func TestLocalServiceLimitsWithinWindow(t *testing.T) {
	service := NewLocalService()
	service.now = func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }
	for attempt := 0; attempt < 5; attempt++ {
		decision, err := service.Allow(context.Background(), "login:ip", 5, time.Minute)
		if err != nil || !decision.Allowed {
			t.Fatalf("attempt %d = (%+v, %v), want allowed", attempt+1, decision, err)
		}
	}
	decision, err := service.Allow(context.Background(), "login:ip", 5, time.Minute)
	if err != nil || decision.Allowed || decision.RetryAfter != time.Minute {
		t.Fatalf("sixth attempt = (%+v, %v), want rejected for one minute", decision, err)
	}
}

func TestLocalServiceEvictsExpiredWindows(t *testing.T) {
	service := NewLocalService()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return base }

	// A thousand distinct clients that each send one request and never return.
	for client := range 1000 {
		if _, err := service.Allow(context.Background(), "api:"+strconv.Itoa(client), 5, time.Minute); err != nil {
			t.Fatalf("Allow() error = %v", err)
		}
	}
	if len(service.windows) != 1000 {
		t.Fatalf("tracked windows = %d, want 1000 while every window is live", len(service.windows))
	}

	// Past both the window and the sweep interval every entry is dead weight.
	service.now = func() time.Time { return base.Add(2 * time.Minute) }
	if _, err := service.Allow(context.Background(), "api:current", 5, time.Minute); err != nil {
		t.Fatalf("Allow() error = %v", err)
	}
	if len(service.windows) != 1 {
		t.Fatalf("tracked windows after sweep = %d, want only the current key", len(service.windows))
	}
}

func TestLocalServiceSweepKeepsLiveWindowsEnforced(t *testing.T) {
	service := NewLocalService()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return base }

	for attempt := range 5 {
		if decision, err := service.Allow(context.Background(), "login:ip", 5, time.Hour); err != nil || !decision.Allowed {
			t.Fatalf("attempt %d = (%+v, %v), want allowed", attempt+1, decision, err)
		}
	}

	// The sweep runs on this call. A window that is still open must survive it,
	// otherwise eviction would silently reset an attacker's attempt budget.
	service.now = func() time.Time { return base.Add(2 * time.Minute) }
	decision, err := service.Allow(context.Background(), "login:ip", 5, time.Hour)
	if err != nil || decision.Allowed {
		t.Fatalf("attempt after sweep = (%+v, %v), want still rejected", decision, err)
	}
}
