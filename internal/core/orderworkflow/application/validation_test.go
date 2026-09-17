package application

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
	workflowDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/core/orderworkflow/domain"
	ordersDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/orders/domain"
)

// This file covers the validation that stands between a provider's word and
// the atomic transaction that acts on it. Every case here asserts that a
// malformed input never reaches the repository — reaching it is what would
// mark an order paid on a confirmation the service should have refused.

func TestAPaymentConfirmationIsRefusedBeforeItReachesTheDatabase(t *testing.T) {
	orderID := uuid.New()
	valid := paymentConfirmation(orderID)

	for name, mutate := range map[string]func(*workflowDomain.PaymentConfirmation){
		"no order": func(c *workflowDomain.PaymentConfirmation) { c.OrderID = uuid.Nil },
		"no provider": func(c *workflowDomain.PaymentConfirmation) {
			c.Provider = "  "
		},
		"no provider reference": func(c *workflowDomain.PaymentConfirmation) {
			// Without it a refund or a dispute cannot be traced to a charge.
			c.ProviderReference = ""
		},
		// A negative amount is not reachable here: money.NewMoney refuses to
		// construct one, so the value object has already ruled it out.
		"zero amount": func(c *workflowDomain.PaymentConfirmation) { c.Amount = mustMoney(0, "EUR") },
		"unconstructed amount": func(c *workflowDomain.PaymentConfirmation) {
			// A zero-value Money has no currency, which Validate rejects.
			c.Amount = money.Money{}
		},
		"wrong status": func(c *workflowDomain.PaymentConfirmation) {
			// A 'failed' body must not be able to drive MarkPaid.
			c.Status = "failed"
		},
	} {
		t.Run(name, func(t *testing.T) {
			repository := &fakeRepository{}
			confirmation := valid
			mutate(&confirmation)

			if err := NewService(repository).MarkPaid(context.Background(), confirmation); err == nil {
				t.Fatal("MarkPaid() error = nil, want the confirmation refused")
			}
			if repository.paid.OrderID != uuid.Nil {
				t.Fatal("an invalid confirmation reached the database and marked the order paid")
			}
		})
	}
}

func TestEachPaymentOutcomeAcceptsOnlyItsOwnStatus(t *testing.T) {
	// The status is the caller's statement of what happened. Accepting a
	// mismatched one would let a failure be recorded as a refund.
	orderID := uuid.New()
	for name, testCase := range map[string]struct {
		call   func(*Service, context.Context, workflowDomain.PaymentConfirmation) error
		status string
	}{
		"paid":     {status: "paid", call: (*Service).MarkPaid},
		"failed":   {status: "failed", call: (*Service).MarkFailed},
		"refunded": {status: "refunded", call: (*Service).MarkRefunded},
	} {
		t.Run(name, func(t *testing.T) {
			confirmation := paymentConfirmation(orderID)
			confirmation.Status = testCase.status
			if err := testCase.call(NewService(&fakeRepository{}), context.Background(), confirmation); err != nil {
				t.Fatalf("matching status rejected: %v", err)
			}
			for _, other := range []string{"paid", "failed", "refunded", "", "PAID"} {
				if other == testCase.status {
					continue
				}
				confirmation.Status = other
				if err := testCase.call(NewService(&fakeRepository{}), context.Background(), confirmation); err == nil {
					t.Fatalf("status %q accepted by the %s outcome", other, name)
				}
			}
		})
	}
}

func TestReservationIDsMustBeUsableBeforeAnOrderIsCreated(t *testing.T) {
	for name, reservationIDs := range map[string][]uuid.UUID{
		"none":      {},
		"nil entry": {uuid.Nil},
		// A duplicate would release the same reservation twice on cancel.
		"duplicate": {func() uuid.UUID { id := uuid.New(); return id }()},
	} {
		t.Run(name, func(t *testing.T) {
			if name == "duplicate" {
				reservationIDs = append(reservationIDs, reservationIDs[0])
			}
			repository := &fakeRepository{}
			if _, err := NewService(repository).CreatePending(context.Background(), validDraft(t), reservationIDs); err == nil {
				t.Fatal("CreatePending() error = nil, want the reservations refused")
			}
			if repository.created != nil {
				t.Fatal("an order was created against unusable reservations")
			}
		})
	}
}

func TestACheckoutRequiresContactDetailsAndAWellFormedAttempt(t *testing.T) {
	// Checkout is where an order acquires someone to send the receipt to. An
	// order created without that is unrecoverable: the confirmation has
	// nowhere to go and the buyer has no record of the purchase.
	reservationIDs := []uuid.UUID{uuid.New()}

	for name, testCase := range map[string]struct {
		draft   func(ordersDomain.Draft) ordersDomain.Draft
		attempt func(workflowDomain.CheckoutAttemptRequest) workflowDomain.CheckoutAttemptRequest
	}{
		"no contact": {draft: func(d ordersDomain.Draft) ordersDomain.Draft { d.Contact = nil; return d }},
		"blank email": {draft: func(d ordersDomain.Draft) ordersDomain.Draft {
			d.Contact = &ordersDomain.ContactDetails{Email: "   ", Locale: "uk"}
			return d
		}},
		"blank locale": {draft: func(d ordersDomain.Draft) ordersDomain.Draft {
			// The locale picks the receipt template; blank renders nothing.
			d.Contact = &ordersDomain.ContactDetails{Email: "buyer@example.com", Locale: ""}
			return d
		}},
		"attempt already bound to an order": {attempt: func(a workflowDomain.CheckoutAttemptRequest) workflowDomain.CheckoutAttemptRequest {
			a.OrderID = uuid.New()
			return a
		}},
		"no provider": {attempt: func(a workflowDomain.CheckoutAttemptRequest) workflowDomain.CheckoutAttemptRequest {
			a.Provider = ""
			return a
		}},
		"no idempotency key": {attempt: func(a workflowDomain.CheckoutAttemptRequest) workflowDomain.CheckoutAttemptRequest {
			// Without it a retried checkout charges twice.
			a.IdempotencyKey = " "
			return a
		}},
		"zero amount": {attempt: func(a workflowDomain.CheckoutAttemptRequest) workflowDomain.CheckoutAttemptRequest {
			a.Amount = mustMoney(0, "EUR")
			return a
		}},
	} {
		t.Run(name, func(t *testing.T) {
			draft := checkoutDraft(t)
			attempt := checkoutAttempt()
			if testCase.draft != nil {
				draft = testCase.draft(draft)
			}
			if testCase.attempt != nil {
				attempt = testCase.attempt(attempt)
			}
			repository := &fakeRepository{}

			if _, err := NewService(repository).CreatePendingCheckout(context.Background(), draft, reservationIDs, attempt); err == nil {
				t.Fatal("CreatePendingCheckout() error = nil, want the checkout refused")
			}
			if repository.created != nil {
				t.Fatal("an order was created from an invalid checkout")
			}
		})
	}
}

func TestAValidCheckoutBindsTheAttemptToTheOrderItCreated(t *testing.T) {
	repository := &fakeRepository{}
	order, err := NewService(repository).CreatePendingCheckout(context.Background(), checkoutDraft(t), []uuid.UUID{uuid.New()}, checkoutAttempt())
	if err != nil {
		t.Fatalf("CreatePendingCheckout() error = %v", err)
	}
	// The attempt arrives unbound and the service is what ties it to the new
	// order; a payment confirmation is matched back through that link.
	if repository.attempt.OrderID != order.ID {
		t.Fatalf("attempt order = %v, want the created order %v", repository.attempt.OrderID, order.ID)
	}
	if repository.attempt.ExpiresAt.IsZero() {
		t.Fatal("the attempt was stored with no expiry, so nothing would ever reclaim it")
	}
}

func TestAFreeCheckoutRefusesAnythingThatIsNotFree(t *testing.T) {
	// The free path skips the provider entirely, so a non-zero amount here
	// would produce a paid order that was never charged.
	for name, mutate := range map[string]func(*workflowDomain.CheckoutAttemptRequest){
		"priced":             func(a *workflowDomain.CheckoutAttemptRequest) { a.Amount = mustMoney(1, "EUR") },
		"different provider": func(a *workflowDomain.CheckoutAttemptRequest) { a.Provider = "stripe" },
		"no idempotency key": func(a *workflowDomain.CheckoutAttemptRequest) { a.IdempotencyKey = "" },
		"bound to an order":  func(a *workflowDomain.CheckoutAttemptRequest) { a.OrderID = uuid.New() },
	} {
		t.Run(name, func(t *testing.T) {
			attempt := workflowDomain.CheckoutAttemptRequest{Provider: "free", IdempotencyKey: "checkout-1", Amount: mustMoney(0, "EUR")}
			mutate(&attempt)
			repository := &fakeRepository{}

			if _, err := NewService(repository).CreatePaidCheckout(context.Background(), checkoutDraft(t), []uuid.UUID{uuid.New()}, attempt); err == nil {
				t.Fatal("CreatePaidCheckout() error = nil, want the attempt refused")
			}
			if repository.created != nil {
				t.Fatal("a paid order was created from an invalid free checkout")
			}
		})
	}
}

func TestAFreeCheckoutReturnsAPaidOrder(t *testing.T) {
	repository := &fakeRepository{}
	attempt := workflowDomain.CheckoutAttemptRequest{Provider: "free", IdempotencyKey: "checkout-1", Amount: mustMoney(0, "EUR")}

	order, err := NewService(repository).CreatePaidCheckout(context.Background(), checkoutDraft(t), []uuid.UUID{uuid.New()}, attempt)
	if err != nil {
		t.Fatalf("CreatePaidCheckout() error = %v", err)
	}
	if order.Status != ordersDomain.StatusPaid {
		t.Fatalf("order status = %q, want %q", order.Status, ordersDomain.StatusPaid)
	}
}

func TestAnOperationalTransitionCarriesWhoDidItAndWhy(t *testing.T) {
	// This is an audited state change. A transition with no actor, no trigger
	// or no event id cannot be reconstructed afterwards, so it is refused
	// rather than recorded anonymously.
	valid := workflowDomain.OperationalStatusTransition{
		OrderID: uuid.New(), ToStatusCode: "shipped", Trigger: "admin",
		ActorType: "admin_user", EventID: uuid.New(),
	}
	for name, mutate := range map[string]func(*workflowDomain.OperationalStatusTransition){
		"no order":      func(tr *workflowDomain.OperationalStatusTransition) { tr.OrderID = uuid.Nil },
		"no target":     func(tr *workflowDomain.OperationalStatusTransition) { tr.ToStatusCode = " " },
		"no trigger":    func(tr *workflowDomain.OperationalStatusTransition) { tr.Trigger = "" },
		"no actor type": func(tr *workflowDomain.OperationalStatusTransition) { tr.ActorType = "" },
		"no event id":   func(tr *workflowDomain.OperationalStatusTransition) { tr.EventID = uuid.Nil },
	} {
		t.Run(name, func(t *testing.T) {
			transition := valid
			mutate(&transition)
			if err := NewService(&fakeRepository{}).TransitionOperational(context.Background(), transition); err == nil {
				t.Fatal("TransitionOperational() error = nil, want the transition refused")
			}
		})
	}

	repository := &fakeRepository{}
	if err := NewService(repository).TransitionOperational(context.Background(), valid); err != nil {
		t.Fatalf("TransitionOperational() error = %v", err)
	}
	if repository.transition.OccurredAt.IsZero() {
		t.Fatal("the transition was recorded with no timestamp")
	}
}

func TestAnAdminCancellationRequiresAnActorAndAReason(t *testing.T) {
	valid := workflowDomain.AdminCancellation{OrderID: uuid.New(), ActorID: uuid.New(), Reason: "customer request"}
	for name, mutate := range map[string]func(*workflowDomain.AdminCancellation){
		"no order":  func(c *workflowDomain.AdminCancellation) { c.OrderID = uuid.Nil },
		"no actor":  func(c *workflowDomain.AdminCancellation) { c.ActorID = uuid.Nil },
		"no reason": func(c *workflowDomain.AdminCancellation) { c.Reason = "   " },
	} {
		t.Run(name, func(t *testing.T) {
			cancellation := valid
			mutate(&cancellation)
			repository := &fakeRepository{}
			if err := NewService(repository).CancelPendingWithActor(context.Background(), cancellation); err == nil {
				t.Fatal("CancelPendingWithActor() error = nil, want the cancellation refused")
			}
			if repository.cancelled != uuid.Nil {
				t.Fatal("an unattributable cancellation reached the database")
			}
		})
	}

	// A repository that does not implement the audited variant must refuse
	// rather than silently fall back to the unattributed cancel.
	if err := NewService(unauditedRepository{&fakeRepository{}}).CancelPendingWithActor(context.Background(), valid); err == nil ||
		!strings.Contains(err.Error(), "not supported") {
		t.Fatalf("error = %v, want the unsupported repository refused", err)
	}
}

func TestAnAdminCancellationGetsAnEventIDWhenTheCallerOmitsOne(t *testing.T) {
	// The event id is the outbox idempotency key, so a missing one would make
	// the audit entry undeduplicable rather than absent.
	repository := &fakeRepository{}
	if err := NewService(repository).CancelPendingWithActor(context.Background(),
		workflowDomain.AdminCancellation{OrderID: uuid.New(), ActorID: uuid.New(), Reason: "customer request"}); err != nil {
		t.Fatalf("CancelPendingWithActor() error = %v", err)
	}
	if repository.cancellation.EventID == uuid.Nil {
		t.Fatal("the cancellation was recorded with no event id")
	}
}

func TestCheckoutAttemptLookupsRejectEmptyAndUnboundedInputs(t *testing.T) {
	service := NewService(&fakeRepository{})
	ctx := context.Background()

	if _, err := service.FindCheckoutAttempt(ctx, "  "); err == nil {
		t.Fatal("FindCheckoutAttempt() error = nil, want a blank key refused")
	}
	// A non-positive age or lease would claim every attempt, including ones
	// another worker is holding.
	if _, err := service.ClaimPendingCheckoutAttempt(ctx, 0, time.Minute); err == nil {
		t.Fatal("ClaimPendingCheckoutAttempt() accepted a zero age")
	}
	if _, err := service.ClaimPendingCheckoutAttempt(ctx, time.Minute, 0); err == nil {
		t.Fatal("ClaimPendingCheckoutAttempt() accepted a zero lease")
	}
	if _, err := service.ExpirePendingCheckout(ctx, time.Time{}); err == nil {
		t.Fatal("ExpirePendingCheckout() accepted a zero time")
	}
	if _, err := service.CurrentStatusForUpdate(ctx, uuid.Nil); err == nil {
		t.Fatal("CurrentStatusForUpdate() accepted a nil order id")
	}
	if err := service.MarkCheckoutAttemptFailed(ctx, uuid.Nil); err == nil {
		t.Fatal("MarkCheckoutAttemptFailed() accepted a nil order id")
	}
	if err := service.RetryCheckoutAttempt(ctx, uuid.New(), nil); err == nil {
		t.Fatal("RetryCheckoutAttempt() accepted a retry with no cause")
	}
	if err := service.CancelPending(ctx, uuid.Nil); err == nil {
		t.Fatal("CancelPending() accepted a nil order id")
	}
	if err := service.RecordCheckoutAttempt(ctx, workflowDomain.CheckoutAttemptRequest{}); err == nil {
		t.Fatal("RecordCheckoutAttempt() accepted an empty request")
	}
}

func TestARepositoryFailureIsReportedRatherThanSwallowed(t *testing.T) {
	failure := errors.New("database unavailable")
	repository := &fakeRepository{createErr: failure}

	if _, err := NewService(repository).CreatePending(context.Background(), validDraft(t), []uuid.UUID{uuid.New()}); !errors.Is(err, failure) {
		t.Fatalf("CreatePending() error = %v, want the repository failure", err)
	}
}

func checkoutDraft(t *testing.T) ordersDomain.Draft {
	t.Helper()
	draft := validDraft(t)
	draft.Contact = &ordersDomain.ContactDetails{Email: "buyer@example.com", Locale: "uk"}
	return draft
}

func checkoutAttempt() workflowDomain.CheckoutAttemptRequest {
	return workflowDomain.CheckoutAttemptRequest{Provider: "stripe", IdempotencyKey: "checkout-1", Amount: mustMoney(1000, "EUR")}
}

// unauditedRepository implements the plain workflow contract but not the
// audited cancellation one, which is the shape of a deployment without the
// admin module. The embedded field is the interface, not the fake: embedding
// the concrete type would promote its CancelPendingWithActor and this would
// satisfy the very interface it is here to lack.
type unauditedRepository struct{ workflowDomain.Repository }
