package stripe

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	paymentsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/payments/domain"
)

func TestCreateCheckoutCreatesIdempotentPaymentIntent(t *testing.T) {
	var form url.Values
	var idempotencyKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/payment_intents" || r.Method != http.MethodPost {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if username, password, ok := r.BasicAuth(); !ok || username != "sk_test" || password != "" {
			t.Fatalf("basic auth = %q:%q, ok=%v", username, password, ok)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		form, idempotencyKey = r.PostForm, r.Header.Get("Idempotency-Key")
		_, _ = w.Write([]byte(`{"id":"pi_123","client_secret":"pi_123_secret_456"}`))
	}))
	defer server.Close()

	adapter := newAdapter(t, server.URL)
	amount := mustMoney(12345, "EUR")
	orderID := uuid.New()
	session, err := adapter.CreateCheckout(context.Background(), paymentsDomain.CheckoutPayment{OrderID: orderID, IdempotencyKey: "checkout-123", Amount: amount})
	if err != nil {
		t.Fatalf("CreateCheckout() error = %v", err)
	}
	if session.ProviderReference != "pi_123" || session.ClientSecret != "pi_123_secret_456" || session.RedirectURL != "" {
		t.Fatalf("session = %+v", session)
	}
	if form.Get("amount") != "12345" || form.Get("currency") != "eur" || form.Get("metadata[order_id]") != orderID.String() || form.Get("automatic_payment_methods[enabled]") != "true" || idempotencyKey != "checkout-123" {
		t.Fatalf("form = %v, idempotency key = %q", form, idempotencyKey)
	}
}

func TestVerifyWebhookChecksRawPayloadSignatureAndMapsPaidEvent(t *testing.T) {
	adapter := newAdapter(t, "https://api.example.test")
	now := time.Date(2026, time.August, 3, 12, 0, 0, 0, time.UTC)
	adapter.now = func() time.Time { return now }
	orderID := uuid.New()
	payload := []byte(fmt.Sprintf(`{"id":"evt_123","type":"payment_intent.succeeded","created":%d,"data":{"object":{"id":"pi_123","amount_received":12345,"currency":"eur","metadata":{"order_id":"%s"}}}}`, now.Unix(), orderID))
	event, err := adapter.VerifyWebhook(context.Background(), paymentsDomain.WebhookRequest{Headers: map[string]string{"stripe-signature": signature("whsec_test", now.Unix(), payload)}, Payload: payload})
	if err != nil {
		t.Fatalf("VerifyWebhook() error = %v", err)
	}
	if event.EventID != "evt_123" || event.OrderID != orderID || event.ProviderReference != "pi_123" || event.Status != "paid" || event.Amount.Amount() != 12345 || event.Amount.Currency() != "EUR" {
		t.Fatalf("event = %+v", event)
	}

	_, err = adapter.VerifyWebhook(context.Background(), paymentsDomain.WebhookRequest{Headers: map[string]string{"Stripe-Signature": "t=1,v1=bad"}, Payload: payload})
	if err != paymentsDomain.ErrInvalidWebhookSignature {
		t.Fatalf("VerifyWebhook() error = %v, want invalid signature", err)
	}
}

func TestVerifyWebhookUsesIntentAmountForFailedPayment(t *testing.T) {
	adapter := newAdapter(t, "https://api.example.test")
	now := time.Date(2026, time.August, 3, 12, 0, 0, 0, time.UTC)
	adapter.now = func() time.Time { return now }
	orderID := uuid.New()
	payload := []byte(fmt.Sprintf(`{"id":"evt_failed","type":"payment_intent.payment_failed","created":%d,"data":{"object":{"id":"pi_123","amount":12345,"amount_received":0,"currency":"eur","metadata":{"order_id":"%s"}}}}`, now.Unix(), orderID))
	event, err := adapter.VerifyWebhook(context.Background(), paymentsDomain.WebhookRequest{Headers: map[string]string{"Stripe-Signature": signature("whsec_test", now.Unix(), payload)}, Payload: payload})
	if err != nil {
		t.Fatalf("VerifyWebhook() error = %v", err)
	}
	if event.Status != "failed" || event.Amount.Amount() != 12345 || event.Amount.Currency() != "EUR" {
		t.Fatalf("event = %+v", event)
	}
}

func TestRefundUsesPaymentIntentAndIdempotencyKey(t *testing.T) {
	var form url.Values
	var idempotencyKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/refunds" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		form, idempotencyKey = r.PostForm, r.Header.Get("Idempotency-Key")
		_, _ = io.WriteString(w, `{"id":"re_123"}`)
	}))
	defer server.Close()

	adapter := newAdapter(t, server.URL)
	amount := mustMoney(500, "EUR")
	if err := adapter.Refund(context.Background(), paymentsDomain.RefundRequest{PaymentReference: "pi_123", Amount: amount, IdempotencyKey: "refund-123"}); err != nil {
		t.Fatalf("Refund() error = %v", err)
	}
	if form.Get("payment_intent") != "pi_123" || form.Get("amount") != "500" || idempotencyKey != "refund-123" {
		t.Fatalf("form = %v, idempotency key = %q", form, idempotencyKey)
	}
}

func newAdapter(t *testing.T, apiURL string) *Adapter {
	t.Helper()
	adapter, err := New(Config{SecretKey: "sk_test", WebhookSecret: "whsec_test", APIURL: apiURL, WebhookTolerance: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	return adapter
}

func signature(secret string, timestamp int64, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(fmt.Sprintf("%d.", timestamp)))
	_, _ = mac.Write(payload)
	return fmt.Sprintf("t=%d,v1=%s", timestamp, hex.EncodeToString(mac.Sum(nil)))
}

func TestNewRejectsMissingCredentials(t *testing.T) {
	if _, err := New(Config{}); err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("New() error = %v", err)
	}
}
