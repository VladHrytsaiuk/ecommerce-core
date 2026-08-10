package ratelimit

import (
	"context"
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
