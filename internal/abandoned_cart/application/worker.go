package application

import (
	"context"
	cart "github.com/VladHrytsaiuk/ecommerce-core/internal/abandoned_cart/domain"
	notifications "github.com/VladHrytsaiuk/ecommerce-core/internal/notifications/domain"
	"github.com/google/uuid"
	"time"
)

const finalizationTimeout = 3 * time.Second

type Policy struct {
	Delays                  []time.Duration
	RequireMarketingConsent bool
	QuietHours              func(time.Time) (time.Time, bool)
}
type Worker struct {
	repo      cart.Repository
	carts     cart.CartRecoveryReader
	consent   cart.MarketingConsentReader
	scheduler notifications.NotificationScheduler
	tx        cart.TransactionManager
	policy    Policy
	now       func() time.Time
}

func NewWorker(r cart.Repository, c cart.CartRecoveryReader, co cart.MarketingConsentReader, n notifications.NotificationScheduler, tx cart.TransactionManager, p Policy) *Worker {
	return &Worker{r, c, co, n, tx, p, func() time.Time { return time.Now().UTC() }}
}
func (w *Worker) Run(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		if c, e := w.repo.ClaimDue(ctx, w.now()); e == nil && c != nil {
			if e = w.process(ctx, c); e != nil {
				// Claiming and processing are deliberately separate transactions.
				// On shutdown the processing tx rolls back, so use a short detached
				// context to release the lease immediately instead of waiting for its
				// expiry. The update is conditional on status=processing.
				finalizeCtx, cancel := finalizationContext(ctx)
				_ = w.repo.Requeue(finalizeCtx, c.ID)
				cancel()
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func finalizationContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), finalizationTimeout)
}
func (w *Worker) process(ctx context.Context, c *cart.Campaign) error {
	if len(w.policy.Delays) == 0 || c.Step < 1 || c.Step > len(w.policy.Delays) {
		return w.repo.Requeue(ctx, c.ID)
	}
	s, e := w.carts.GetCartState(ctx, c.CartID)
	if e != nil {
		return e
	}
	now := w.now()
	return w.tx.WithinTransaction(ctx, func(tc context.Context) error {
		if s.IsPaid || !s.IsActive || s.IsEmpty {
			c.Status = "converted"
			return w.repo.Update(tc, c)
		}
		if d := w.policy.Delays[c.Step-1]; s.LastUpdatedAt.Add(d).After(now) {
			c.DueAt = s.LastUpdatedAt.Add(d)
			c.Status = "pending"
			return w.repo.Update(tc, c)
		}
		if until, quiet := w.policy.QuietHours(now); quiet {
			c.DueAt = until
			c.Status = "pending"
			return w.repo.Update(tc, c)
		}
		if w.policy.RequireMarketingConsent {
			consentGranted, err := w.consent.HasConsent(tc, c.CustomerID, c.ContactEmail)
			if err != nil {
				return err
			}
			if !consentGranted {
				c.Status = "skipped"
				return w.repo.Update(tc, c)
			}
		}
		c.Status = "scheduled"
		if e := w.repo.Update(tc, c); e != nil {
			return e
		}
		if e := w.scheduler.ScheduleEmail(tc, "abandoned_cart", c.ContactEmail, struct {
			CartID uuid.UUID `json:"cart_id"`
			Step   int       `json:"step"`
		}{c.CartID, c.Step}); e != nil {
			return e
		}
		if c.Step < len(w.policy.Delays) {
			return w.repo.Create(tc, cart.Campaign{ID: uuid.New(), CartID: c.CartID, CustomerID: c.CustomerID, ContactEmail: c.ContactEmail, Step: c.Step + 1, Status: "pending", DueAt: now.Add(w.policy.Delays[c.Step])})
		}
		return nil
	})
}
