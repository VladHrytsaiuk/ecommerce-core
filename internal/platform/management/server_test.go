package management

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestServerLivenessAndReadiness(t *testing.T) {
	server := NewServer("127.0.0.1:0", ReadinessCheck{Name: "postgres", Check: func(_ context.Context) error { return nil }})

	live := httptest.NewRecorder()
	server.server.Handler.ServeHTTP(live, httptest.NewRequest(http.MethodGet, "/livez", nil))
	if live.Code != http.StatusOK {
		t.Fatalf("live status = %d, want %d", live.Code, http.StatusOK)
	}

	ready := httptest.NewRecorder()
	server.server.Handler.ServeHTTP(ready, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if ready.Code != http.StatusOK {
		t.Fatalf("ready status = %d, want %d", ready.Code, http.StatusOK)
	}
}

func TestServerReadinessFailsClosed(t *testing.T) {
	server := NewServer("127.0.0.1:0", ReadinessCheck{Name: "redis", Check: func(_ context.Context) error { return errors.New("down") }})

	recorder := httptest.NewRecorder()
	server.server.Handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("ready status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
}
