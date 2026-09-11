package application

import (
	"context"
	cart "github.com/VladHrytsaiuk/ecommerce-core/internal/abandoned_cart/domain"
	notifications "github.com/VladHrytsaiuk/ecommerce-core/internal/notifications/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/worker"
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
	logger    worker.Logger
}

// WithLogger reports a failing pass. A recovery campaign that stops running
// produces no error anyone sees — only revenue that quietly does not arrive.
func (w *Worker) WithLogger(logger worker.Logger) *Worker {
	if w != nil && logger != nil {
		w.logger = logger
	}
	return w
}

func NewWorker(r cart.Repository, c cart.CartRecoveryReader, co cart.MarketingConsentReader, n notifications.NotificationScheduler, tx cart.TransactionManager, p Policy) *Worker {
	return &Worker{repo: r, carts: c, consent: co, scheduler: n, tx: tx, policy: p, now: func() time.Time { return time.Now().UTC() }}
}

// Run drains the due campaigns each tick. One campaign per tick meant the
// queue was paced by the ticker rather than by how much work was waiting.
func (w *Worker) Run(ctx context.Context, interval time.Duration) {
	worker.LoopDraining(ctx, interval, time.Minute, w.logger, "abandoned cart worker", w.claimAndProcess)
}

func (w *Worker) claimAndProcess(ctx context.Context) (bool, error) {
	campaign, err := w.repo.ClaimDue(ctx, w.now())
	if err != nil || campaign == nil {
		return false, err
	}
	if err := w.process(ctx, campaign); err != nil {
		// Claiming and processing are deliberately separate transactions. On
		// shutdown the processing tx rolls back, so use a short detached
		// context to release the lease immediately instead of waiting for its
		// expiry. The update is conditional on status=processing.
		finalizeCtx, cancel := finalizationContext(ctx)
		requeueErr := w.repo.Requeue(finalizeCtx, campaign.ID)
		cancel()
		if requeueErr != nil {
			return true, requeueErr
		}
		// The campaign is durably back in the queue, so this pass is healthy
		// and the rest of the backlog must not wait for the next tick.
		return true, nil
	}
	return true, nil
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
		// A recovery campaign records the contact email and nothing about
		// the shopper's language, so the store's locale is used.
		if e := w.scheduler.ScheduleEmail(tc, "abandoned_cart", "", c.ContactEmail, struct {
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
