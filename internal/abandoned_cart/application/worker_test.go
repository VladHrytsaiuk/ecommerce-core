package application

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
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
	requeueToken           uuid.UUID
	leaseLost              bool
	updatedStatuses        []string
	createdSteps           []int
}

func (f *workerRepoFake) ClaimDue(context.Context, time.Time) (*cart.Campaign, error) {
	item := f.claimed
	f.claimed = nil
	return item, nil
}

// Update models the fenced write: once the lease is gone it refuses, exactly
// as the repository's conditional UPDATE does when it matches no row.
func (f *workerRepoFake) Update(_ context.Context, x *cart.Campaign) error {
	if f.leaseLost {
		return cart.ErrLeaseLost
	}
	f.updatedStatuses = append(f.updatedStatuses, x.Status)
	return nil
}
func (f *workerRepoFake) Create(_ context.Context, x cart.Campaign) error {
	f.createdSteps = append(f.createdSteps, x.Step)
	return nil
}
func (*workerRepoFake) CreateOrReset(context.Context, cart.Campaign) error { return nil }
func (f *workerRepoFake) Requeue(ctx context.Context, _, token uuid.UUID) error {
	f.requeued = true
	f.requeueContextCanceled = ctx.Err() != nil
	f.requeueToken = token
	return nil
}

// workerCartDue reports a cart that is still active and overdue, which is the
// state that carries a pass all the way to scheduling an email and creating
// the next step.
type workerCartDue struct{}

func (workerCartDue) GetCartState(context.Context, uuid.UUID) (cart.CartState, error) {
	return cart.CartState{IsActive: true, LastUpdatedAt: time.Now().Add(-48 * time.Hour)}, nil
}

type workerCartFail struct{}

func (workerCartFail) GetCartState(context.Context, uuid.UUID) (cart.CartState, error) {
	return cart.CartState{}, errors.New("db unavailable")
}

type workerConsentFake struct{}

func (workerConsentFake) HasConsent(context.Context, *uuid.UUID, string) (bool, error) {
	return false, nil
}

type workerSchedulerFake struct {
	locales []string
	payload any
}

func (f *workerSchedulerFake) ScheduleEmail(_ context.Context, _, locale, _ string, payload any) error {
	f.locales = append(f.locales, locale)
	f.payload = payload
	return nil
}

type workerTxFake struct{}

func (workerTxFake) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

// TestALostLeaseStopsThePassBeforeAnySideEffect is the defect the token was
// added for and did not actually prevent. The conditional UPDATE matched no
// row and returned nil, so the worker read that as a successful write and went
// on to schedule the recovery email and create the next campaign step — both
// of which the worker that had taken the campaign over was about to do itself.
//
// The visible result was worse than a duplicate email: the new holder then hit
// the unique constraint on (cart_id, step) creating a step that already
// existed, failed its pass, and returned the campaign to the queue — where it
// could be picked up and fail the same way again.
func TestALostLeaseStopsThePassBeforeAnySideEffect(t *testing.T) {
	repository := &workerRepoFake{
		claimed:   &cart.Campaign{ID: uuid.New(), CartID: uuid.New(), ContactEmail: "buyer@example.com", Step: 1, LockToken: uuid.New()},
		leaseLost: true,
	}
	scheduler := &workerSchedulerFake{}
	worker := NewWorker(repository, workerCartDue{}, workerConsentFake{}, scheduler, workerTxFake{},
		Policy{Delays: []time.Duration{time.Hour, 2 * time.Hour}, QuietHours: func(time.Time) (time.Time, bool) { return time.Time{}, false }}, stubLinker{})

	worked, err := worker.claimAndProcess(context.Background())
	if err != nil {
		t.Fatalf("claimAndProcess() error = %v; a lease taken over is a race, not a fault", err)
	}
	if !worked {
		t.Fatal("claimAndProcess() reported no work although it claimed a campaign")
	}
	if len(scheduler.locales) != 0 {
		t.Fatalf("%d emails scheduled by a worker that no longer owns the campaign", len(scheduler.locales))
	}
	if len(repository.createdSteps) != 0 {
		t.Fatalf("next steps created = %v; the holder creates those, and a duplicate collides on (cart_id, step)", repository.createdSteps)
	}
	if repository.requeued {
		t.Fatal("the campaign was released by a worker that no longer holds it")
	}
}

// TestAHeldLeaseCompletesTheWholePass is the other direction: the guard must
// not stop a worker that still owns the campaign.
func TestAHeldLeaseCompletesTheWholePass(t *testing.T) {
	repository := &workerRepoFake{
		claimed: &cart.Campaign{ID: uuid.New(), CartID: uuid.New(), ContactEmail: "buyer@example.com", Step: 1, LockToken: uuid.New()},
	}
	scheduler := &workerSchedulerFake{}
	worker := NewWorker(repository, workerCartDue{}, workerConsentFake{}, scheduler, workerTxFake{},
		Policy{Delays: []time.Duration{time.Hour, 2 * time.Hour}, QuietHours: func(time.Time) (time.Time, bool) { return time.Time{}, false }}, stubLinker{})

	if _, err := worker.claimAndProcess(context.Background()); err != nil {
		t.Fatalf("claimAndProcess() error = %v", err)
	}
	if len(scheduler.locales) != 1 {
		t.Fatalf("%d emails scheduled, want 1", len(scheduler.locales))
	}
	if len(repository.createdSteps) != 1 || repository.createdSteps[0] != 2 {
		t.Fatalf("next steps created = %v, want [2]", repository.createdSteps)
	}
	if len(repository.updatedStatuses) != 1 || repository.updatedStatuses[0] != "scheduled" {
		t.Fatalf("statuses written = %v, want [scheduled]", repository.updatedStatuses)
	}
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
	worker := NewWorker(repository, workerCartFail{}, workerConsentFake{}, &workerSchedulerFake{}, workerTxFake{}, Policy{Delays: []time.Duration{time.Hour}, QuietHours: func(time.Time) (time.Time, bool) { return time.Time{}, false }}, stubLinker{})
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

func TestAFailedPassReleasesTheClaimWithItsOwnToken(t *testing.T) {
	// Releasing on the campaign id alone would let a worker whose lease had
	// already been taken over hand the campaign back while another worker is
	// still running it, so two passes would overlap.
	claim := &cart.Campaign{ID: uuid.New(), CartID: uuid.New(), Step: 1, LockToken: uuid.New()}
	repository := &workerRepoFake{claimed: claim}
	worker := NewWorker(repository, workerCartFail{}, workerConsentFake{}, &workerSchedulerFake{}, workerTxFake{},
		Policy{Delays: []time.Duration{time.Hour}, QuietHours: func(time.Time) (time.Time, bool) { return time.Time{}, false }}, stubLinker{})

	if _, err := worker.claimAndProcess(context.Background()); err != nil {
		t.Fatalf("claimAndProcess() error = %v", err)
	}
	if !repository.requeued {
		t.Fatal("the failed pass did not release its claim")
	}
	if repository.requeueToken != claim.LockToken {
		t.Fatalf("released with token %s, want the claim's own %s", repository.requeueToken, claim.LockToken)
	}
}

func TestWorkerDoesNotStartClaimingDuringShutdown(t *testing.T) {
	repository := &workerRepoFake{claimed: &cart.Campaign{ID: uuid.New(), CartID: uuid.New(), Step: 1}}
	worker := NewWorker(repository, workerCartFail{}, workerConsentFake{}, &workerSchedulerFake{}, workerTxFake{}, Policy{Delays: []time.Duration{time.Hour}, QuietHours: func(time.Time) (time.Time, bool) { return time.Time{}, false }}, stubLinker{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	worker.Run(ctx, time.Millisecond)

	if repository.claimed == nil {
		t.Fatal("a campaign was claimed on an already-cancelled context")
	}
}

var _ notifications.NotificationScheduler = (*workerSchedulerFake)(nil)

// stubLinker stands in for Consent's signed opt-out capability.
type stubLinker struct{ err error }

func (l stubLinker) URLFor(email string) (string, error) {
	if l.err != nil {
		return "", l.err
	}
	return "https://store.example/unsubscribe?token=signed-for-" + email, nil
}

func TestAMarketingEmailCarriesAWayOut(t *testing.T) {
	// A guest has no account, so the link in the message is the only way they
	// can stop receiving these. The template renders it, so the payload has to
	// carry it.
	repository := &workerRepoFake{claimed: &cart.Campaign{ID: uuid.New(), CartID: uuid.New(), ContactEmail: "guest@example.test", Step: 1, LockToken: uuid.New()}}
	scheduler := &workerSchedulerFake{}
	worker := NewWorker(repository, workerCartDue{}, workerConsentFake{}, scheduler, workerTxFake{},
		Policy{Delays: []time.Duration{time.Hour}, QuietHours: func(time.Time) (time.Time, bool) { return time.Time{}, false }}, stubLinker{})

	if _, err := worker.claimAndProcess(context.Background()); err != nil {
		t.Fatalf("claimAndProcess() error = %v", err)
	}
	if len(scheduler.locales) != 1 {
		t.Fatalf("scheduled %d emails, want one", len(scheduler.locales))
	}
	encoded, err := json.Marshal(scheduler.payload)
	if err != nil {
		t.Fatal(err)
	}
	var fields struct {
		UnsubscribeURL string `json:"unsubscribe_url"`
	}
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fields.UnsubscribeURL, "guest@example.test") {
		t.Fatalf("unsubscribe_url = %q, want a link issued for this recipient", fields.UnsubscribeURL)
	}
}

func TestAMarketingEmailIsNotSentWithoutAWayOut(t *testing.T) {
	// If the capability is misconfigured the message must not go at all. A
	// marketing email a recipient cannot leave is worse than one not sent.
	repository := &workerRepoFake{claimed: &cart.Campaign{ID: uuid.New(), CartID: uuid.New(), ContactEmail: "guest@example.test", Step: 1, LockToken: uuid.New()}}
	scheduler := &workerSchedulerFake{}
	worker := NewWorker(repository, workerCartDue{}, workerConsentFake{}, scheduler, workerTxFake{},
		Policy{Delays: []time.Duration{time.Hour}, QuietHours: func(time.Time) (time.Time, bool) { return time.Time{}, false }},
		stubLinker{err: errors.New("unsubscribe capability is not configured")})

	// The pass itself is healthy — the campaign goes back in the queue and the
	// worker moves on, which is how every other processing failure is handled.
	// What must not happen is the message going out anyway.
	if _, err := worker.claimAndProcess(context.Background()); err != nil {
		t.Fatalf("claimAndProcess() error = %v", err)
	}
	if len(scheduler.locales) != 0 {
		t.Fatal("a marketing email was scheduled that the recipient cannot leave")
	}
	if !repository.requeued {
		t.Fatal("the campaign was neither sent nor returned to the queue")
	}
	if len(repository.createdSteps) != 0 {
		t.Fatal("the campaign advanced a step without its message being sent")
	}
}
