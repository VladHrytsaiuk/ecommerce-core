package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
	workflowDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/core/orderworkflow/domain"
	ordersDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/orders/domain"
	paymentsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/payments/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
)

func TestRecoveryRegistersClaimedCheckoutUsingStableIdempotencyKey(t *testing.T) {
	attempt := recoveryAttempt(t)
	workflow := &recoveryWorkflow{claimed: &attempt}
	gateway := &recoveryGateway{session: paymentsDomain.PaymentSession{ProviderReference: "pi_recovered"}}

	err := NewRecoveryService(workflow, recoveryGateways{"recovery": gateway}, recoveryLogger{}).RecoverOnce(context.Background(), time.Minute, time.Minute)
	if err != nil {
		t.Fatalf("RecoverOnce() error = %v", err)
	}
	if gateway.payment.IdempotencyKey != attempt.IdempotencyKey || workflow.registered.ProviderReference != "pi_recovered" || workflow.retried != uuid.Nil {
		t.Fatalf("gateway=%+v registered=%+v retried=%s", gateway.payment, workflow.registered, workflow.retried)
	}
}

func TestRecoveryRetriesTransientGatewayFailure(t *testing.T) {
	attempt := recoveryAttempt(t)
	workflow := &recoveryWorkflow{claimed: &attempt}
	errGateway := errors.New("gateway timeout")

	err := NewRecoveryService(workflow, recoveryGateways{"recovery": &recoveryGateway{err: errGateway}}, recoveryLogger{}).RecoverOnce(context.Background(), time.Minute, time.Minute)
	if err != nil {
		t.Fatalf("RecoverOnce() error = %v", err)
	}
	if workflow.retried != attempt.OrderID || !errors.Is(workflow.retryCause, errGateway) || workflow.cancelled != uuid.Nil {
		t.Fatalf("workflow=%+v", workflow)
	}
}

func TestRecoveryCancelsExplicitlyRejectedCheckout(t *testing.T) {
	attempt := recoveryAttempt(t)
	workflow := &recoveryWorkflow{claimed: &attempt}

	err := NewRecoveryService(workflow, recoveryGateways{"recovery": &recoveryGateway{err: paymentsDomain.ErrGatewayRejected}}, recoveryLogger{}).RecoverOnce(context.Background(), time.Minute, time.Minute)
	if err != nil {
		t.Fatalf("RecoverOnce() error = %v", err)
	}
	if workflow.failed != attempt.OrderID || workflow.cancelled != attempt.OrderID || workflow.retried != uuid.Nil {
		t.Fatalf("workflow=%+v", workflow)
	}
}

func recoveryAttempt(t *testing.T) workflowDomain.CheckoutAttempt {
	t.Helper()
	amount, err := money.New(1000, "EUR")
	if err != nil {
		t.Fatal(err)
	}
	return workflowDomain.CheckoutAttempt{OrderID: uuid.New(), Provider: "recovery", IdempotencyKey: "stable-checkout-key", Amount: amount}
}

type recoveryWorkflow struct {
	claimed                    *workflowDomain.CheckoutAttempt
	registered                 workflowDomain.PaymentAttempt
	failed, cancelled, retried uuid.UUID
	retryCause                 error
}

func (w *recoveryWorkflow) ClaimPendingCheckoutAttempt(context.Context, time.Duration, time.Duration) (*workflowDomain.CheckoutAttempt, error) {
	return w.claimed, nil
}
func (w *recoveryWorkflow) RegisterPayment(_ context.Context, attempt workflowDomain.PaymentAttempt) error {
	w.registered = attempt
	return nil
}
func (w *recoveryWorkflow) MarkCheckoutAttemptFailed(_ context.Context, id uuid.UUID) error {
	w.failed = id
	return nil
}
func (w *recoveryWorkflow) RetryCheckoutAttempt(_ context.Context, id uuid.UUID, cause error) error {
	w.retried, w.retryCause = id, cause
	return nil
}
func (w *recoveryWorkflow) CreatePending(context.Context, ordersDomain.Draft, []uuid.UUID) (*ordersDomain.Order, error) {
	return nil, nil
}
func (*recoveryWorkflow) CreatePendingCheckout(context.Context, ordersDomain.Draft, []uuid.UUID, workflowDomain.CheckoutAttemptRequest) (*ordersDomain.Order, error) {
	return nil, nil
}
func (w *recoveryWorkflow) RecordCheckoutAttempt(context.Context, workflowDomain.CheckoutAttemptRequest) error {
	return nil
}
func (*recoveryWorkflow) FindCheckoutAttempt(context.Context, string) (*workflowDomain.CheckoutAttempt, error) {
	return nil, nil
}
func (w *recoveryWorkflow) CancelPending(_ context.Context, id uuid.UUID) error {
	w.cancelled = id
	return nil
}
func (*recoveryWorkflow) MarkPaid(context.Context, workflowDomain.PaymentConfirmation) error {
	return nil
}
func (*recoveryWorkflow) MarkFailed(context.Context, workflowDomain.PaymentConfirmation) error {
	return nil
}

type recoveryGateway struct {
	payment paymentsDomain.CheckoutPayment
	session paymentsDomain.PaymentSession
	err     error
}
type recoveryGateways map[string]paymentsDomain.Gateway

func (g recoveryGateways) Get(code string) (paymentsDomain.Gateway, bool) {
	gateway, ok := g[code]
	return gateway, ok
}

func (*recoveryGateway) Code() string { return "recovery" }
func (g *recoveryGateway) CreateCheckout(_ context.Context, payment paymentsDomain.CheckoutPayment) (paymentsDomain.PaymentSession, error) {
	g.payment = payment
	return g.session, g.err
}
func (*recoveryGateway) VerifyWebhook(context.Context, paymentsDomain.WebhookRequest) (paymentsDomain.PaymentEvent, error) {
	return paymentsDomain.PaymentEvent{}, nil
}
func (*recoveryGateway) Refund(context.Context, paymentsDomain.RefundRequest) error { return nil }

type recoveryLogger struct{}

func (recoveryLogger) Debug(string, ...zap.Field)      {}
func (recoveryLogger) Info(string, ...zap.Field)       {}
func (recoveryLogger) Warn(string, ...zap.Field)       {}
func (recoveryLogger) Error(string, ...zap.Field)      {}
func (recoveryLogger) Fatal(string, ...zap.Field)      {}
func (recoveryLogger) Debugf(string, ...interface{})   {}
func (recoveryLogger) Infof(string, ...interface{})    {}
func (recoveryLogger) Warnf(string, ...interface{})    {}
func (recoveryLogger) Errorf(string, ...interface{})   {}
func (recoveryLogger) Fatalf(string, ...interface{})   {}
func (recoveryLogger) Debugw(string, ...interface{})   {}
func (recoveryLogger) Infow(string, ...interface{})    {}
func (recoveryLogger) Warnw(string, ...interface{})    {}
func (recoveryLogger) Errorw(string, ...interface{})   {}
func (recoveryLogger) Fatalw(string, ...interface{})   {}
func (recoveryLogger) With(...zap.Field) logger.Logger { return recoveryLogger{} }
func (recoveryLogger) Sync() error                     { return nil }

var _ recoveryWorkflowPort = (*recoveryWorkflow)(nil)
var _ paymentsDomain.Gateway = (*recoveryGateway)(nil)
