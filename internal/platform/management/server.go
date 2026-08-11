// Package management provides a private operational HTTP endpoint. It is
// intentionally separate from the public Gin router.
package management

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type ReadinessCheck struct {
	Name  string
	Check func(context.Context) error
}

type Server struct {
	server  *http.Server
	checks  []ReadinessCheck
	mu      sync.Mutex
	started bool
}

func NewServer(addr string, checks ...ReadinessCheck) *Server {
	s := &Server{checks: checks}
	mux := http.NewServeMux()
	mux.HandleFunc("/livez", s.live)
	mux.HandleFunc("/readyz", s.ready)
	mux.Handle("/metrics", promhttp.Handler())
	s.server = &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	return s
}

// Start binds the listener synchronously so an invalid management address is
// observable immediately; serving continues in its own goroutine.
func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		return nil
	}
	listener, err := net.Listen("tcp", s.server.Addr)
	if err != nil {
		return err
	}
	s.started = true
	go func() {
		if err := s.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			// There is no process logger dependency here. A terminated listener
			// is surfaced by readiness probes and application shutdown.
		}
	}()
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	started := s.started
	s.mu.Unlock()
	if !started {
		return nil
	}
	return s.server.Shutdown(ctx)
}

func (s *Server) live(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	for _, check := range s.checks {
		if check.Check == nil {
			continue
		}
		if err := check.Check(ctx); err != nil {
			http.Error(w, fmt.Sprintf("%s unavailable", check.Name), http.StatusServiceUnavailable)
			return
		}
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ready\n"))
}
