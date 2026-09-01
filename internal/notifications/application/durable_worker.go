package application

import (
	"context"
	"encoding/json"
	notifications "github.com/VladHrytsaiuk/ecommerce-core/internal/notifications/domain"
	"time"
)

type DurableWorker struct {
	store    notifications.DurableJobStore
	renderer notifications.TemplateRenderer
	sender   notifications.EmailSender
	max      int
}

func NewDurableWorker(s notifications.DurableJobStore, r notifications.TemplateRenderer, e notifications.EmailSender) *DurableWorker {
	return &DurableWorker{store: s, renderer: r, sender: e, max: 5}
}
func (w *DurableWorker) DispatchOnce(ctx context.Context) error {
	j, err := w.store.ClaimDue(ctx, time.Now().UTC())
	if err != nil || j == nil {
		return err
	}
	var payload any
	if json.Unmarshal(j.Payload, &payload) != nil {
		return w.store.Fail(context.WithoutCancel(ctx), *j, time.Now().UTC(), true)
	}
	m, err := w.renderer.Render(ctx, j.Type, "en", payload)
	if err == nil {
		m.MessageID = j.ID.String()
		m.To = j.Email
		_, err = w.sender.Send(ctx, m)
	}
	if err == nil {
		return w.store.Complete(context.WithoutCancel(ctx), *j)
	}
	n := time.Now().UTC().Add(time.Second * time.Duration(1<<min(j.RetryCount, 6)))
	return w.store.Fail(context.WithoutCancel(ctx), *j, n, j.RetryCount >= w.max)
}
func (w *DurableWorker) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		_ = w.DispatchOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
