// Package cache contains provider-neutral caching ports.
package cache

import (
	"context"
	"errors"
	"time"
)

var ErrMiss = errors.New("cache miss")

// Service is deliberately byte-oriented so domain/application packages choose
// their own serialization and never depend on a cache provider SDK.
type Service interface {
	Set(context.Context, string, []byte, time.Duration) error
	Get(context.Context, string) ([]byte, error)
	Delete(context.Context, string) error
	DeleteByPrefix(context.Context, string) error
}
