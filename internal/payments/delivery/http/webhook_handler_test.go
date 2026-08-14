package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
	workflowDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/core/orderworkflow/domain"
	ordersDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/orders/domain"
	paymentsApp "github.com/VladHrytsaiuk/ecommerce-core/internal/payments/application"
	paymentsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/payments/domain"
)

func TestWebhookHandlerRegistersGenericProviderRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	registry, _ := paymentsApp.NewRegistry([]string{"fake"}, "fake", httpGateway{})
	router := gin.New()
	RegisterWebhookRoutes(router.Group("/api"), paymentsApp.NewWebhookService(registry, &httpEventStore{}, &httpWorkflow{}))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/webhooks/payments/fake", nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

type httpGateway struct{}

func (httpGateway) Code() string { return "fake" }
func (httpGateway) CreateCheckout(context.Context, paymentsDomain.CheckoutPayment) (paymentsDomain.PaymentSession, error) {
	return paymentsDomain.PaymentSession{}, nil
}
func (httpGateway) VerifyWebhook(context.Context, paymentsDomain.WebhookRequest) (paymentsDomain.PaymentEvent, error) {
	amount, _ := money.New(1, "EUR")
	return paymentsDomain.PaymentEvent{EventID: "event", OrderID: uuid.New(), Status: "pending", Amount: amount}, nil
}
func (httpGateway) Refund(context.Context, paymentsDomain.RefundRequest) error { return nil }

type httpEventStore struct{}

func (*httpEventStore) Claim(context.Context, string, paymentsDomain.PaymentEvent) (bool, error) {
	return true, nil
}
func (*httpEventStore) MarkProcessed(context.Context, string, string) error { return nil }
func (*httpEventStore) Abandon(context.Context, string, string) error       { return nil }

type httpWorkflow struct{}

func (*httpWorkflow) CreatePending(context.Context, ordersDomain.Draft, []uuid.UUID) (*ordersDomain.Order, error) {
	return nil, nil
}
func (*httpWorkflow) CreatePendingCheckout(context.Context, ordersDomain.Draft, []uuid.UUID, workflowDomain.CheckoutAttemptRequest) (*ordersDomain.Order, error) {
	return nil, nil
}
func (*httpWorkflow) CreatePaidCheckout(context.Context, ordersDomain.Draft, []uuid.UUID, workflowDomain.CheckoutAttemptRequest) (*ordersDomain.Order, error) {
	return nil, nil
}
func (*httpWorkflow) RegisterPayment(context.Context, workflowDomain.PaymentAttempt) error {
	return nil
}
func (*httpWorkflow) RecordCheckoutAttempt(context.Context, workflowDomain.CheckoutAttemptRequest) error {
	return nil
}
func (*httpWorkflow) FindCheckoutAttempt(context.Context, string) (*workflowDomain.CheckoutAttempt, error) {
	return nil, nil
}
func (*httpWorkflow) ClaimPendingCheckoutAttempt(context.Context, time.Duration, time.Duration) (*workflowDomain.CheckoutAttempt, error) {
	return nil, nil
}
func (*httpWorkflow) ExpirePendingCheckout(context.Context, time.Time) (bool, error) {
	return false, nil
}
func (*httpWorkflow) MarkCheckoutAttemptFailed(context.Context, uuid.UUID) error         { return nil }
func (*httpWorkflow) RetryCheckoutAttempt(context.Context, uuid.UUID, error) error       { return nil }
func (*httpWorkflow) MarkPaid(context.Context, workflowDomain.PaymentConfirmation) error { return nil }
func (*httpWorkflow) MarkFailed(context.Context, workflowDomain.PaymentConfirmation) error {
	return nil
}
func (*httpWorkflow) MarkRefunded(context.Context, workflowDomain.PaymentConfirmation) error { return nil }
func (*httpWorkflow) CancelPending(context.Context, uuid.UUID) error { return nil }

var _ workflowDomain.Service = (*httpWorkflow)(nil)
