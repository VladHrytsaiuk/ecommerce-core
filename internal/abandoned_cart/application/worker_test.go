package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	cart "github.com/VladHrytsaiuk/ecommerce-core/internal/abandoned_cart/domain"
	notifications "github.com/VladHrytsaiuk/ecommerce-core/internal/notifications/domain"
)

type workerRepoFake struct {
	claimed                *cart.Campaign
	requeued               bool
	requeueContextCanceled bool
}

func (f *workerRepoFake) ClaimDue(context.Context, time.Time) (*cart.Campaign, error) {
	item := f.claimed
	f.claimed = nil
	return item, nil
}
func (*workerRepoFake) Update(context.Context, *cart.Campaign) error       { return nil }
func (*workerRepoFake) Create(context.Context, cart.Campaign) error        { return nil }
func (*workerRepoFake) CreateOrReset(context.Context, cart.Campaign) error { return nil }
func (f *workerRepoFake) Requeue(ctx context.Context, _ uuid.UUID) error {
	f.requeued = true
	f.requeueContextCanceled = ctx.Err() != nil
	return nil
}

type workerCartFail struct{}

func (workerCartFail) GetCartState(context.Context, uuid.UUID) (cart.CartState, error) {
	return cart.CartState{}, errors.New("db unavailable")
}

type workerConsentFake struct{}

func (workerConsentFake) HasConsent(context.Context, *uuid.UUID, string) (bool, error) {
	return false, nil
}

type workerSchedulerFake struct{ locales []string }

func (f *workerSchedulerFake) ScheduleEmail(_ context.Context, _, locale, _ string, _ any) error {
	f.locales = append(f.locales, locale)
	return nil
}

type workerTxFake struct{}

func (workerTxFake) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

func TestWorkerRequeuesClaimWithFinalizationContextAfterCancellation(t *testing.T) {
	// Claiming and processing are separate transactions, so a campaign whose
	// processing failed during shutdown has to have its lease released on a
	// detached context — the cancelled one would roll the release back too and
	// the campaign would sit unclaimable until its lease expired.
	//
	// This drives the pass directly rather than through Run, because Run does
	// no work on an already-cancelled context: shutdown is not the time to
	// start claiming.
	repository := &workerRepoFake{claimed: &cart.Campaign{ID: uuid.New(), CartID: uuid.New(), Step: 1}}
	worker := NewWorker(repository, workerCartFail{}, workerConsentFake{}, &workerSchedulerFake{}, workerTxFake{}, Policy{Delays: []time.Duration{time.Hour}, QuietHours: func(time.Time) (time.Time, bool) { return time.Time{}, false }})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := worker.claimAndProcess(ctx); err != nil {
		t.Fatalf("claimAndProcess() error = %v", err)
	}
	if !repository.requeued {
		t.Fatal("claimed campaign was not requeued")
	}
	if repository.requeueContextCanceled {
		t.Fatal("requeue must use detached finalization context")
	}
}

func TestWorkerDoesNotStartClaimingDuringShutdown(t *testing.T) {
	repository := &workerRepoFake{claimed: &cart.Campaign{ID: uuid.New(), CartID: uuid.New(), Step: 1}}
	worker := NewWorker(repository, workerCartFail{}, workerConsentFake{}, &workerSchedulerFake{}, workerTxFake{}, Policy{Delays: []time.Duration{time.Hour}, QuietHours: func(time.Time) (time.Time, bool) { return time.Time{}, false }})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	worker.Run(ctx, time.Millisecond)

	if repository.claimed == nil {
		t.Fatal("a campaign was claimed on an already-cancelled context")
	}
}

var _ notifications.NotificationScheduler = (*workerSchedulerFake)(nil)
