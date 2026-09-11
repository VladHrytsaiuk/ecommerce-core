package application

import (
	"context"
	"encoding/json"
	notifications "github.com/VladHrytsaiuk/ecommerce-core/internal/notifications/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/worker"
	"time"
)

type DurableWorker struct {
	store    notifications.DurableJobStore
	renderer notifications.TemplateRenderer
	sender   notifications.EmailSender
	max      int
	logger   worker.Logger
}

func NewDurableWorker(s notifications.DurableJobStore, r notifications.TemplateRenderer, e notifications.EmailSender) *DurableWorker {
	return &DurableWorker{store: s, renderer: r, sender: e, max: 5}
}

// WithLogger reports a failing pass. Email that stops going out is the kind of
// failure a store notices days later from customer complaints, so the worker
// saying so at the time is the difference.
func (w *DurableWorker) WithLogger(logger worker.Logger) *DurableWorker {
	if w != nil && logger != nil {
		w.logger = logger
	}
	return w
}

// DispatchOnce sends at most one scheduled email.
func (w *DurableWorker) DispatchOnce(ctx context.Context) error {
	_, err := w.dispatchOnce(ctx)
	return err
}

func (w *DurableWorker) dispatchOnce(ctx context.Context) (bool, error) {
	j, err := w.store.ClaimDue(ctx, time.Now().UTC())
	if err != nil || j == nil {
		return false, err
	}
	var payload any
	if json.Unmarshal(j.Payload, &payload) != nil {
		return true, w.store.Fail(context.WithoutCancel(ctx), *j, time.Now().UTC(), true)
	}
	m, err := w.renderer.Render(ctx, j.Type, j.Locale, payload)
	if err == nil {
		m.MessageID = j.ID.String()
		m.To = j.Email
		_, err = w.sender.Send(ctx, m)
	}
	if err == nil {
		return true, w.store.Complete(context.WithoutCancel(ctx), *j)
	}
	n := time.Now().UTC().Add(time.Second * time.Duration(1<<min(j.RetryCount, 6)))
	return true, w.store.Fail(context.WithoutCancel(ctx), *j, n, j.RetryCount >= w.max)
}

// Run drains the scheduled queue each tick. One email per five-second tick
// meant a back-in-stock announcement to a few hundred subscribers took most of
// an hour to go out.
func (w *DurableWorker) Run(ctx context.Context, interval time.Duration) {
	worker.LoopDraining(ctx, interval, 5*time.Second, w.logger, "notification worker", w.dispatchOnce)
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
