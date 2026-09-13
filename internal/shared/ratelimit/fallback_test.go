package ratelimit

import (
	"context"
	"errors"
	"testing"
	"time"
)

// The distributed limiter guards login and all public browsing through the same
// middleware, and an error from it refuses the request. That is right for login
// and wrong for a catalog page: a Redis blip returned 503 for the entire
// versioned API. These hold the split.

func TestAHealthyPrimaryDecidesAlone(t *testing.T) {
	primary := &stubService{decision: Decision{Allowed: true}}
	standby := &stubService{}
	fallback := NewFallback(primary, standby, nil)

	decision, err := fallback.Allow(context.Background(), "key", 10, time.Minute)

	if err != nil || !decision.Allowed {
		t.Fatalf("Allow() = (%+v, %v)", decision, err)
	}
	if standby.calls != 0 {
		t.Fatal("the standby was consulted while the primary was healthy")
	}
}

func TestAPrimaryRefusalIsNotOverriddenByTheStandby(t *testing.T) {
	// Degrading must not become a way around the limit. A client the
	// distributed limiter has cut off stays cut off.
	primary := &stubService{decision: Decision{Allowed: false, RetryAfter: 30 * time.Second}}
	standby := &stubService{decision: Decision{Allowed: true}}
	fallback := NewFallback(primary, standby, nil)

	decision, err := fallback.Allow(context.Background(), "key", 10, time.Minute)

	if err != nil {
		t.Fatalf("Allow() error = %v", err)
	}
	if decision.Allowed {
		t.Fatal("a refusal from the distributed limiter was overturned by the local one")
	}
	if decision.RetryAfter != 30*time.Second {
		t.Fatalf("RetryAfter = %v, want the primary's", decision.RetryAfter)
	}
	if standby.calls != 0 {
		t.Fatal("the standby was consulted for a decision the primary made")
	}
}

func TestAnUnreachablePrimaryDegradesInsteadOfFailing(t *testing.T) {
	// This is the whole point: the storefront keeps serving.
	primary := &stubService{err: errors.New("dial tcp: connection refused")}
	standby := &stubService{decision: Decision{Allowed: true}}
	fallback := NewFallback(primary, standby, nil)

	decision, err := fallback.Allow(context.Background(), "key", 10, time.Minute)

	if err != nil {
		t.Fatalf("Allow() error = %v; a cache outage must not fail the request", err)
	}
	if !decision.Allowed || standby.calls != 1 {
		t.Fatalf("decision = %+v after %d standby calls", decision, standby.calls)
	}
}

func TestTheLimitStillAppliesWhileDegraded(t *testing.T) {
	// Degraded is per-process, not absent. A client that exhausts the local
	// window is still refused.
	primary := &stubService{err: errors.New("unreachable")}
	fallback := NewFallback(primary, NewLocalService(), nil)

	var refused bool
	for range 5 {
		decision, err := fallback.Allow(context.Background(), "client", 2, time.Minute)
		if err != nil {
			t.Fatalf("Allow() error = %v", err)
		}
		if !decision.Allowed {
			refused = true
		}
	}
	if !refused {
		t.Fatal("no ceiling at all while degraded")
	}
}

func TestAnOutageIsAnnouncedOnceAndSoIsRecovery(t *testing.T) {
	// One line per transition. Logging every request during an outage buries
	// the line that says the outage began.
	primary := &stubService{err: errors.New("unreachable")}
	recorder := &recordingLogger{}
	fallback := NewFallback(primary, NewLocalService(), recorder)

	for range 5 {
		_, _ = fallback.Allow(context.Background(), "client", 100, time.Minute)
	}
	if recorder.warnings != 1 {
		t.Fatalf("%d warnings for one outage, want 1", recorder.warnings)
	}

	primary.err = nil
	for range 5 {
		_, _ = fallback.Allow(context.Background(), "client", 100, time.Minute)
	}
	if recorder.infos != 1 {
		t.Fatalf("%d recovery lines, want 1", recorder.infos)
	}

	// A second outage is announced again, or a flapping dependency would go
	// unreported after the first time.
	primary.err = errors.New("unreachable again")
	_, _ = fallback.Allow(context.Background(), "client", 100, time.Minute)
	if recorder.warnings != 2 {
		t.Fatalf("%d warnings after a second outage, want 2", recorder.warnings)
	}
}

func TestAMiswiredFallbackRefusesRatherThanAllows(t *testing.T) {
	// A decorator built without its parts must not become an open door.
	for name, fallback := range map[string]*Fallback{
		"no primary": NewFallback(nil, NewLocalService(), nil),
		"no standby": NewFallback(&stubService{}, nil, nil),
		"nil":        nil,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := fallback.Allow(context.Background(), "key", 10, time.Minute); err == nil {
				t.Fatal("a miswired limiter allowed the request")
			}
		})
	}
}

type stubService struct {
	decision Decision
	err      error
	calls    int
}

func (s *stubService) Allow(context.Context, string, int, time.Duration) (Decision, error) {
	s.calls++
	if s.err != nil {
		return Decision{}, s.err
	}
	return s.decision, nil
}

type recordingLogger struct{ warnings, infos int }

func (l *recordingLogger) Warnw(string, ...interface{}) { l.warnings++ }
func (l *recordingLogger) Infow(string, ...interface{}) { l.infos++ }
