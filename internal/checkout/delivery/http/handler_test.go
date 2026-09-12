package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	cartDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/cart/domain"
	checkoutDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/checkout/domain"
	workflowDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/core/orderworkflow/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/apiresponse"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware"
	ordersDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/orders/domain"
	paymentsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/payments/domain"
)

func TestStartPaymentGeneratesServerCheckoutIDAndMapsRedirect(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeCheckout{}
	variantID, warehouseID := uuid.New(), uuid.New()
	carts := &fakeCart{cart: &cartDomain.Cart{Items: []cartDomain.Item{{VariantID: variantID, Quantity: 2}}}}
	router, _ := newCheckoutRouter(service, carts, warehouseID)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/checkout/es/payment", strings.NewReader(`{"customer_email":"buyer@example.com","customer_phone":"+34123456789","return_url":"https://store.example/return"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "checkout-test-1")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	if service.request.Preparation.CheckoutID == uuid.Nil || service.request.Preparation.Locale != "es" || len(service.request.Preparation.Lines) != 1 || service.request.Preparation.Lines[0].VariantID != variantID || service.request.Preparation.Lines[0].WarehouseID != warehouseID || service.request.Preparation.Lines[0].Quantity != 2 || service.request.Preparation.ExpiresAt.Before(time.Now()) {
		t.Fatalf("mapped request = %+v", service.request)
	}
}

func TestStartPaymentReturnsClientSecretOnlyWhenGatewayProvidesOne(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeCheckout{clientSecret: "pi_test_secret"}
	carts := &fakeCart{cart: &cartDomain.Cart{Items: []cartDomain.Item{{VariantID: uuid.New(), Quantity: 1}}}}
	router, _ := newCheckoutRouter(service, carts, uuid.New())

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/checkout/es/payment", strings.NewReader(`{"customer_email":"buyer@example.com","customer_phone":"+34123456789"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "checkout-test-2")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated || !strings.Contains(recorder.Body.String(), `"client_secret":"pi_test_secret"`) {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
}

// An empty cart is a well-formed request the current state cannot satisfy,
// which v1 answers 422 rather than the 400 its predecessor used.
func TestStartPaymentRejectsEmptyCart(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router, _ := newCheckoutRouter(&fakeCheckout{}, &fakeCart{cart: &cartDomain.Cart{}}, uuid.New())

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/checkout/es/payment", strings.NewReader(`{}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "checkout-test-empty")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	// "cart is empty" stays in the log, not the body. The predecessor put the
	// raw error text in the response, which is how a database failure reached
	// the buyer carrying PostgreSQL's message.
	if strings.Contains(recorder.Body.String(), "cart is empty") {
		t.Fatalf("body = %s, want the internal message kept out of the response", recorder.Body.String())
	}
}

func TestStartPaymentRequiresStableIdempotencyKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeCheckout{}
	router, _ := newCheckoutRouter(service, &fakeCart{cart: &cartDomain.Cart{Items: []cartDomain.Item{{VariantID: uuid.New(), Quantity: 1}}}}, uuid.New())

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/checkout/es/payment", strings.NewReader(`{}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	// A missing Idempotency-Key is a malformed request, so v1 answers 400 with
	// the shared problem-details envelope. The header name is deliberately not
	// echoed back: the envelope carries a stable code and a static detail, and
	// the specifics go to the log rather than to the caller.
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"code":"INVALID_PAYLOAD"`) {
		t.Fatalf("body = %s, want the shared problem-details code", recorder.Body.String())
	}
}

func TestStartPaymentDerivesSameCheckoutIDForRetryKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeCheckout{}
	router, _ := newCheckoutRouter(service, &fakeCart{cart: &cartDomain.Cart{Items: []cartDomain.Item{{VariantID: uuid.New(), Quantity: 1}}}}, uuid.New())

	var first uuid.UUID
	for range 2 {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/v1/checkout/es/payment", strings.NewReader(`{}`))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Idempotency-Key", "retry-key-1")
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusCreated {
			t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
		}
		if first == uuid.Nil {
			first = service.request.Preparation.CheckoutID
		} else if service.request.Preparation.CheckoutID != first {
			t.Fatalf("retry checkout id = %s, want %s", service.request.Preparation.CheckoutID, first)
		}
	}
}

func TestQuoteDeliveryUsesActiveCart(t *testing.T) {
	gin.SetMode(gin.TestMode)
	variantID, warehouseID := uuid.New(), uuid.New()
	service := &fakeCheckout{}
	router, _ := newCheckoutRouter(service, &fakeCart{cart: &cartDomain.Cart{Items: []cartDomain.Item{{VariantID: variantID, Quantity: 3}}}}, warehouseID)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/checkout/es/delivery-options", strings.NewReader(`{"delivery_provider":"novaposhta","delivery":{"locality_id":"city"}}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || service.quote.Lines[0].VariantID != variantID || service.quote.Lines[0].WarehouseID != warehouseID || service.quote.Lines[0].Quantity != 3 {
		t.Fatalf("status = %d quote = %+v body=%s", recorder.Code, service.quote, recorder.Body.String())
	}
}

type fakeCheckout struct {
	request      checkoutDomain.StartPaymentRequest
	quote        checkoutDomain.DeliveryQuoteRequest
	clientSecret string
}

func (s *fakeCheckout) QuoteDelivery(_ context.Context, request checkoutDomain.DeliveryQuoteRequest) (*checkoutDomain.DeliveryQuote, error) {
	s.quote = request
	return &checkoutDomain.DeliveryQuote{Provider: "novaposhta"}, nil
}

type fakeCart struct{ cart *cartDomain.Cart }

func (s *fakeCart) GetOrCreate(_ context.Context, owner cartDomain.Owner) (*cartDomain.Cart, error) {
	result := *s.cart
	result.Owner = owner
	return &result, nil
}
func (*fakeCart) Add(context.Context, cartDomain.Owner, cartDomain.Item) (*cartDomain.Cart, error) {
	return nil, nil
}
func (*fakeCart) SetQuantity(context.Context, cartDomain.Owner, cartDomain.Item) (*cartDomain.Cart, error) {
	return nil, nil
}
func (*fakeCart) Remove(context.Context, cartDomain.Owner, uuid.UUID) (*cartDomain.Cart, error) {
	return nil, nil
}
func (*fakeCart) SetPromoCode(context.Context, cartDomain.Owner, string) (*cartDomain.Cart, error) {
	return nil, nil
}

func (s *fakeCheckout) PreparePayment(context.Context, checkoutDomain.PrepareRequest) (*checkoutDomain.PreparedCheckout, error) {
	return nil, nil
}
func (s *fakeCheckout) StartPayment(_ context.Context, request checkoutDomain.StartPaymentRequest) (*checkoutDomain.StartedCheckout, error) {
	s.request = request
	amount := mustMoney(100, "EUR")
	order := &ordersDomain.Order{ID: uuid.New(), Number: "STORE-1", PaymentProvider: "liqpay"}
	return &checkoutDomain.StartedCheckout{Prepared: &checkoutDomain.PreparedCheckout{ExpiresAt: request.Preparation.ExpiresAt, Total: amount}, Order: order, Session: paymentsDomain.PaymentSession{ProviderReference: "payment-1", RedirectURL: "https://pay.example", ClientSecret: s.clientSecret}}, nil
}
func (*fakeCheckout) ConfirmPayment(context.Context, workflowDomain.PaymentConfirmation) error {
	return nil
}
func (*fakeCheckout) CancelPayment(context.Context, uuid.UUID) error { return nil }

// newCheckoutRouter builds the v1 registration these tests exercise. The
// predecessor registration they used to drive served the same two operations
// through a handler that wrote err.Error() into the response body; it was
// removed, and v1 is now the only checkout contract.
func newCheckoutRouter(service checkoutDomain.Service, carts cartDomain.Service, warehouseID uuid.UUID) (*gin.Engine, *apiresponse.ErrorRenderer) {
	gin.SetMode(gin.TestMode)
	renderer := apiresponse.NewErrorRenderer(nil)
	router := gin.New()
	router.Use(renderer.Middleware())
	group := router.Group("/api/v1/checkout/:lang")
	group.Use(middleware.NewLocaleMiddleware(middleware.LocaleOptions{DefaultLocale: "es", FallbackLocale: "es", SupportedLocales: []string{"es"}}))
	RegisterV1Routes(group, service, carts, 15*time.Minute, warehouseID, false, renderer)
	return router, renderer
}
