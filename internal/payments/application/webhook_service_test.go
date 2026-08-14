package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
	workflowDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/core/orderworkflow/domain"
	ordersDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/orders/domain"
	paymentsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/payments/domain"
)

func TestWebhookServiceProcessesPaidEventExactlyOnce(t *testing.T) {
	orderID := uuid.New()
	gateways, err := NewRegistry([]string{"fake"}, "fake", webhookGateway{event: webhookEvent(orderID, "paid")})
	if err != nil {
		t.Fatal(err)
	}
	events := &fakeEventStore{}
	workflow := &fakeWorkflow{}
	service := NewWebhookService(gateways, events, workflow)
	if err := service.Handle(context.Background(), "fake", paymentsDomain.WebhookRequest{Payload: []byte("callback")}); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if workflow.paid.OrderID != orderID || !events.processed {
		t.Fatalf("workflow=%+v events=%+v", workflow, events)
	}

	events.claimed = false
	if err := service.Handle(context.Background(), "fake", paymentsDomain.WebhookRequest{}); err != nil {
		t.Fatalf("duplicate Handle() error = %v", err)
	}
	if workflow.calls != 1 {
		t.Fatalf("workflow calls = %d, want one", workflow.calls)
	}
}

func TestWebhookServiceAbandonsClaimWhenWorkflowFails(t *testing.T) {
	orderID := uuid.New()
	gateways, err := NewRegistry([]string{"fake"}, "fake", webhookGateway{event: webhookEvent(orderID, "failed")})
	if err != nil {
		t.Fatal(err)
	}
	events := &fakeEventStore{claimed: true}
	service := NewWebhookService(gateways, events, &fakeWorkflow{err: errors.New("workflow unavailable")})
	if err := service.Handle(context.Background(), "fake", paymentsDomain.WebhookRequest{}); err == nil {
		t.Fatal("Handle() error = nil")
	}
	if !events.abandoned {
		t.Fatal("event claim was not abandoned")
	}
}

func TestWebhookServiceReleasesWorkflowForCancelledOrExpiredPayment(t *testing.T) {
	for _, status := range []string{"cancelled", "expired"} {
		t.Run(status, func(t *testing.T) {
			workflow := &fakeWorkflow{}
			service := NewWebhookService(&Registry{gateways: map[string]paymentsDomain.Gateway{"fake": webhookGateway{event: webhookEvent(uuid.New(), status)}}}, &fakeEventStore{}, workflow)
			if err := service.Handle(context.Background(), "fake", paymentsDomain.WebhookRequest{}); err != nil {
				t.Fatalf("Handle() error = %v", err)
			}
			if workflow.failed.Status != "failed" {
				t.Fatalf("failed confirmation = %+v", workflow.failed)
			}
		})
	}
}

func TestWebhookServiceRoutesVerifiedRefundToWorkflow(t *testing.T) {
	t.Parallel()
	orderID := uuid.New()
	workflow := &fakeWorkflow{}
	service := NewWebhookService(&Registry{gateways: map[string]paymentsDomain.Gateway{
		"fake": webhookGateway{event: webhookEvent(orderID, "refunded")},
	}}, &fakeEventStore{}, workflow)
	if err := service.Handle(context.Background(), "fake", paymentsDomain.WebhookRequest{}); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if workflow.refunded.OrderID != orderID || workflow.refunded.Status != "refunded" {
		t.Fatalf("refund confirmation = %+v", workflow.refunded)
	}
}

func webhookEvent(orderID uuid.UUID, status string) paymentsDomain.PaymentEvent {
	amount, _ := money.New(100, "EUR")
	return paymentsDomain.PaymentEvent{EventID: "event-1", OrderID: orderID, Status: status, Amount: amount, OccurredAt: time.Now()}
}

type webhookGateway struct{ event paymentsDomain.PaymentEvent }

func (g webhookGateway) Code() string { return "fake" }
func (g webhookGateway) CreateCheckout(context.Context, paymentsDomain.CheckoutPayment) (paymentsDomain.PaymentSession, error) {
	return paymentsDomain.PaymentSession{}, nil
}
func (g webhookGateway) VerifyWebhook(context.Context, paymentsDomain.WebhookRequest) (paymentsDomain.PaymentEvent, error) {
	return g.event, nil
}
func (g webhookGateway) Refund(context.Context, paymentsDomain.RefundRequest) error { return nil }

type fakeEventStore struct{ claimed, processed, abandoned bool }

func (s *fakeEventStore) Claim(context.Context, string, paymentsDomain.PaymentEvent) (bool, error) {
	return s.claimed || !s.processed, nil
}
func (s *fakeEventStore) MarkProcessed(context.Context, string, string) error {
	s.processed = true
	return nil
}
func (s *fakeEventStore) Abandon(context.Context, string, string) error {
	s.abandoned = true
	return nil
}

type fakeWorkflow struct {
	paid      workflowDomain.PaymentConfirmation
	failed    workflowDomain.PaymentConfirmation
	refunded  workflowDomain.PaymentConfirmation
	cancelled uuid.UUID
	calls     int
	err       error
}

func (w *fakeWorkflow) CreatePending(context.Context, ordersDomain.Draft, []uuid.UUID) (*ordersDomain.Order, error) {
	panic("unused")
}
func (*fakeWorkflow) CreatePendingCheckout(context.Context, ordersDomain.Draft, []uuid.UUID, workflowDomain.CheckoutAttemptRequest) (*ordersDomain.Order, error) {
	panic("unused")
}
func (*fakeWorkflow) RegisterPayment(context.Context, workflowDomain.PaymentAttempt) error {
	return nil
}
func (*fakeWorkflow) RecordCheckoutAttempt(context.Context, workflowDomain.CheckoutAttemptRequest) error {
	return nil
}
func (*fakeWorkflow) FindCheckoutAttempt(context.Context, string) (*workflowDomain.CheckoutAttempt, error) {
	return nil, nil
}
func (*fakeWorkflow) ClaimPendingCheckoutAttempt(context.Context, time.Duration, time.Duration) (*workflowDomain.CheckoutAttempt, error) {
	return nil, nil
}
func (*fakeWorkflow) MarkCheckoutAttemptFailed(context.Context, uuid.UUID) error   { return nil }
func (*fakeWorkflow) RetryCheckoutAttempt(context.Context, uuid.UUID, error) error { return nil }
func (w *fakeWorkflow) MarkPaid(_ context.Context, confirmation workflowDomain.PaymentConfirmation) error {
	w.calls++
	w.paid = confirmation
	return w.err
}
func (w *fakeWorkflow) MarkFailed(_ context.Context, confirmation workflowDomain.PaymentConfirmation) error {
	w.calls++
	w.failed = confirmation
	return w.err
}
func (w *fakeWorkflow) MarkRefunded(_ context.Context, confirmation workflowDomain.PaymentConfirmation) error {
	w.calls++
	w.refunded = confirmation
	return w.err
}
func (w *fakeWorkflow) CancelPending(_ context.Context, orderID uuid.UUID) error {
	w.calls++
	w.cancelled = orderID
	return w.err
}

var _ paymentWorkflow = (*fakeWorkflow)(nil)
var _ paymentsDomain.WebhookEventStore = (*fakeEventStore)(nil)
