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

	checkoutDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/checkout/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware"
	ordersDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/orders/domain"
	paymentsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/payments/domain"
)

func TestStartPaymentGeneratesServerCheckoutIDAndMapsRedirect(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeCheckout{}
	router := gin.New()
	localized := router.Group("/api/:lang")
	localized.Use(middleware.NewLocaleMiddleware(middleware.LocaleOptions{DefaultLocale: "es", FallbackLocale: "es", SupportedLocales: []string{"es"}}))
	RegisterRoutes(localized, service, 15*time.Minute)
	variantID, warehouseID := uuid.New(), uuid.New()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/es/checkout/payment", strings.NewReader(`{"customer_phone":"+34123456789","return_url":"https://store.example/return","lines":[{"variant_id":"`+variantID.String()+`","warehouse_id":"`+warehouseID.String()+`","quantity":2}]}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	if service.request.Preparation.CheckoutID == uuid.Nil || service.request.Preparation.Locale != "es" || len(service.request.Preparation.Lines) != 1 || service.request.Preparation.Lines[0].Quantity != 2 || service.request.Preparation.ExpiresAt.Before(time.Now()) {
		t.Fatalf("mapped request = %+v", service.request)
	}
}

func TestStartPaymentReturnsClientSecretOnlyWhenGatewayProvidesOne(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeCheckout{clientSecret: "pi_test_secret"}
	router := gin.New()
	localized := router.Group("/api/:lang")
	localized.Use(middleware.NewLocaleMiddleware(middleware.LocaleOptions{DefaultLocale: "es", FallbackLocale: "es", SupportedLocales: []string{"es"}}))
	RegisterRoutes(localized, service, 15*time.Minute)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/es/checkout/payment", strings.NewReader(`{"customer_phone":"+34123456789","lines":[{"variant_id":"`+uuid.New().String()+`","warehouse_id":"`+uuid.New().String()+`","quantity":1}]}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated || !strings.Contains(recorder.Body.String(), `"client_secret":"pi_test_secret"`) {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
}

type fakeCheckout struct {
	request      checkoutDomain.StartPaymentRequest
	clientSecret string
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
func (*fakeCheckout) ConfirmPayment(context.Context, uuid.UUID) error { return nil }
func (*fakeCheckout) CancelPayment(context.Context, uuid.UUID) error  { return nil }
