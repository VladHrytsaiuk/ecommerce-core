package app

import (
	"context"
	"errors"
	"io"
	"testing"
)

func TestPlatformRuntimeRollbackReleasesEverythingItAcquired(t *testing.T) {
	first, second := &countingCloser{}, &countingCloser{}
	shutdowns := 0
	runtime := &platformRuntime{
		closers:           []io.Closer{first, second},
		telemetryShutdown: func(context.Context) error { shutdowns++; return nil },
	}

	runtime.rollback()

	// A Bootstrap that fails after opening Redis and a trace exporter must not
	// leak either into a process that is about to exit.
	if first.closed != 1 || second.closed != 1 || shutdowns != 1 {
		t.Fatalf("closes = %d/%d, telemetry shutdowns = %d; want 1/1 and 1", first.closed, second.closed, shutdowns)
	}
}

func TestPlatformRuntimeRollbackIsANoOpOnceCommitted(t *testing.T) {
	closer := &countingCloser{}
	shutdowns := 0
	runtime := &platformRuntime{
		closers:           []io.Closer{closer},
		telemetryShutdown: func(context.Context) error { shutdowns++; return nil },
	}

	runtime.commit()
	// Bootstrap defers rollback unconditionally, so after a successful
	// assembly this must not close the pool the Application is now using.
	runtime.rollback()

	if closer.closed != 0 || shutdowns != 0 {
		t.Fatalf("committed runtime was released: closes = %d, shutdowns = %d", closer.closed, shutdowns)
	}
}

func TestPlatformRuntimeRollbackIsIdempotent(t *testing.T) {
	closer := &countingCloser{}
	runtime := &platformRuntime{closers: []io.Closer{closer}}

	runtime.rollback()
	runtime.rollback()

	if closer.closed != 1 {
		t.Fatalf("closes = %d, want a second rollback to do nothing", closer.closed)
	}
}

func TestPlatformRuntimeAdoptedResourcesAreReleasedToo(t *testing.T) {
	early, late := &countingCloser{}, &countingCloser{}
	runtime := &platformRuntime{closers: []io.Closer{early}}

	// A module that opens a resource part-way through composition registers it
	// here, so a later failure releases it with the rest.
	runtime.adopt(late)
	runtime.adopt(nil)
	runtime.rollback()

	if early.closed != 1 || late.closed != 1 {
		t.Fatalf("closes = %d/%d, want both released", early.closed, late.closed)
	}
}

func TestPlatformRuntimeCloserClosesEveryResourceDespiteAFailure(t *testing.T) {
	failing := &countingCloser{err: errors.New("connection reset")}
	healthy := &countingCloser{}
	runtime := &platformRuntime{closers: []io.Closer{failing, healthy}}

	err := runtime.closer().Close()

	// One bad connection must not strand the others at shutdown, and the
	// failure still has to reach the caller.
	if err == nil || healthy.closed != 1 {
		t.Fatalf("Close() error = %v, healthy closes = %d; want the error surfaced and everything closed", err, healthy.closed)
	}
}

func TestPlatformRuntimeCloserIsNilWhenNothingWasPooled(t *testing.T) {
	// A deployment without Redis has nothing to close, and the Application
	// must not be handed a closer that does nothing.
	if closer := (&platformRuntime{}).closer(); closer != nil {
		t.Fatalf("closer() = %v, want nil for an empty runtime", closer)
	}
}

type countingCloser struct {
	closed int
	err    error
}

func (c *countingCloser) Close() error {
	c.closed++
	return c.err
}
