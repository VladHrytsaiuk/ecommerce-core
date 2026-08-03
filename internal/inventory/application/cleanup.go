package application

import (
	"context"
	"time"
)

type ExpiredReservationStore interface {
	ReleaseExpiredUnattached(context.Context, time.Time, int) (int, error)
}

type Cleanup struct{ store ExpiredReservationStore }

func NewCleanup(store ExpiredReservationStore) *Cleanup { return &Cleanup{store: store} }
func (c *Cleanup) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		_, _ = c.store.ReleaseExpiredUnattached(ctx, time.Now().UTC(), 100)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
