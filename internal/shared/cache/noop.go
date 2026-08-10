package cache

import (
	"context"
	"time"
)

// NoOpService preserves application behaviour when caching is disabled.
type NoOpService struct{}

func NewNoOpService() NoOpService { return NoOpService{} }

func (NoOpService) Set(context.Context, string, []byte, time.Duration) error { return nil }
func (NoOpService) Get(context.Context, string) ([]byte, error)              { return nil, ErrMiss }
func (NoOpService) Delete(context.Context, string) error                     { return nil }
func (NoOpService) DeleteByPrefix(context.Context, string) error             { return nil }
