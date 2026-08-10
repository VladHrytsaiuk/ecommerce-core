// Package ratelimit defines a provider-neutral fixed-window limiter port.
package ratelimit

import (
	"context"
	"sync"
	"time"
)

type Decision struct {
	Allowed    bool
	RetryAfter time.Duration
}

type Service interface {
	Allow(context.Context, string, int, time.Duration) (Decision, error)
}

type localWindow struct {
	count     int
	expiresAt time.Time
}

// LocalService is the explicitly degraded, per-process fallback when Redis is
// disabled. It is safe for concurrent HTTP requests but not distributed.
type LocalService struct {
	mu      sync.Mutex
	windows map[string]localWindow
	now     func() time.Time
}

func NewLocalService() *LocalService {
	return &LocalService{windows: make(map[string]localWindow), now: time.Now}
}

func (s *LocalService) Allow(_ context.Context, key string, limit int, window time.Duration) (Decision, error) {
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.windows[key]
	if !ok || !now.Before(entry.expiresAt) {
		entry = localWindow{expiresAt: now.Add(window)}
	}
	entry.count++
	s.windows[key] = entry
	remaining := entry.expiresAt.Sub(now)
	if remaining < 0 {
		remaining = 0
	}
	return Decision{Allowed: entry.count <= limit, RetryAfter: remaining}, nil
}
