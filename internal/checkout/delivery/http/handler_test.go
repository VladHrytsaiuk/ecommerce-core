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
	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
	workflowDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/core/orderworkflow/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware"
	ordersDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/orders/domain"
	paymentsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/payments/domain"
)

func TestStartPaymentGeneratesServerCheckoutIDAndMapsRedirect(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeCheckout{}
	variantID, warehouseID := uuid.New(), uuid.New()
	carts := &fakeCart{cart: &cartDomain.Cart{Items: []cartDomain.Item{{VariantID: variantID, Quantity: 2}}}}
	router := gin.New()
	localized := router.Group("/api/:lang")
	localized.Use(middleware.NewLocaleMiddleware(middleware.LocaleOptions{DefaultLocale: "es", FallbackLocale: "es", SupportedLocales: []string{"es"}}))
	RegisterRoutes(localized, service, carts, 15*time.Minute, warehouseID)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/es/checkout/payment", strings.NewReader(`{"customer_phone":"+34123456789","return_url":"https://store.example/return"}`))
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
	router := gin.New()
	localized := router.Group("/api/:lang")
	localized.Use(middleware.NewLocaleMiddleware(middleware.LocaleOptions{DefaultLocale: "es", FallbackLocale: "es", SupportedLocales: []string{"es"}}))
	RegisterRoutes(localized, service, carts, 15*time.Minute, uuid.New())

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/es/checkout/payment", strings.NewReader(`{"customer_phone":"+34123456789"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "checkout-test-2")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated || !strings.Contains(recorder.Body.String(), `"client_secret":"pi_test_secret"`) {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestStartPaymentRejectsEmptyCart(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	localized := router.Group("/api/:lang")
	localized.Use(middleware.NewLocaleMiddleware(middleware.LocaleOptions{DefaultLocale: "es", FallbackLocale: "es", SupportedLocales: []string{"es"}}))
	RegisterRoutes(localized, &fakeCheckout{}, &fakeCart{cart: &cartDomain.Cart{}}, 15*time.Minute, uuid.New())

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/es/checkout/payment", strings.NewReader(`{}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "checkout-test-empty")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnprocessableEntity || !strings.Contains(recorder.Body.String(), "cart is empty") {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestStartPaymentRequiresStableIdempotencyKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeCheckout{}
	router := gin.New()
	localized := router.Group("/api/:lang")
	localized.Use(middleware.NewLocaleMiddleware(middleware.LocaleOptions{DefaultLocale: "es", FallbackLocale: "es", SupportedLocales: []string{"es"}}))
	RegisterRoutes(localized, service, &fakeCart{cart: &cartDomain.Cart{Items: []cartDomain.Item{{VariantID: uuid.New(), Quantity: 1}}}}, 15*time.Minute, uuid.New())

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/es/checkout/payment", strings.NewReader(`{}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "Idempotency-Key") {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestStartPaymentDerivesSameCheckoutIDForRetryKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeCheckout{}
	router := gin.New()
	localized := router.Group("/api/:lang")
	localized.Use(middleware.NewLocaleMiddleware(middleware.LocaleOptions{DefaultLocale: "es", FallbackLocale: "es", SupportedLocales: []string{"es"}}))
	RegisterRoutes(localized, service, &fakeCart{cart: &cartDomain.Cart{Items: []cartDomain.Item{{VariantID: uuid.New(), Quantity: 1}}}}, 15*time.Minute, uuid.New())

	var first uuid.UUID
	for range 2 {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/es/checkout/payment", strings.NewReader(`{}`))
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
	router := gin.New()
	localized := router.Group("/api/:lang")
	localized.Use(middleware.NewLocaleMiddleware(middleware.LocaleOptions{DefaultLocale: "es", FallbackLocale: "es", SupportedLocales: []string{"es"}}))
	RegisterRoutes(localized, service, &fakeCart{cart: &cartDomain.Cart{Items: []cartDomain.Item{{VariantID: variantID, Quantity: 3}}}}, 15*time.Minute, warehouseID)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/es/checkout/delivery-options", strings.NewReader(`{"delivery_provider":"novaposhta","delivery":{"locality_id":"city"}}`))
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

func (s *fakeCheckout) PreparePayment(context.Context, checkoutDomain.PrepareRequest) (*checkoutDomain.PreparedCheckout, error) {
	return nil, nil
}
func (s *fakeCheckout) StartPayment(_ context.Context, request checkoutDomain.StartPaymentRequest) (*checkoutDomain.StartedCheckout, error) {
	s.request = request
	amount, _ := money.New(100, "EUR")
	order := &ordersDomain.Order{ID: uuid.New(), Number: "STORE-1", PaymentProvider: "liqpay"}
	return &checkoutDomain.StartedCheckout{Prepared: &checkoutDomain.PreparedCheckout{ExpiresAt: request.Preparation.ExpiresAt, Total: amount}, Order: order, Session: paymentsDomain.PaymentSession{ProviderReference: "payment-1", RedirectURL: "https://pay.example", ClientSecret: s.clientSecret}}, nil
}
func (*fakeCheckout) ConfirmPayment(context.Context, workflowDomain.PaymentConfirmation) error {
	return nil
}
func (*fakeCheckout) CancelPayment(context.Context, uuid.UUID) error { return nil }
