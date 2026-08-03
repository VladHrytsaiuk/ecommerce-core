package liqpay

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
	paymentsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/payments/domain"
)

func TestCreateCheckoutMapsNeutralPaymentToSignedLiqPayURL(t *testing.T) {
	adapter := newAdapter(t, Config{})
	amount, _ := money.New(12345, "EUR")
	orderID := uuid.New()
	session, err := adapter.CreateCheckout(context.Background(), paymentsDomain.CheckoutPayment{OrderID: orderID, IdempotencyKey: "checkout-1", Amount: amount, ReturnURL: "https://store.example/return"})
	if err != nil {
		t.Fatalf("CreateCheckout() error = %v", err)
	}
	checkoutURL, err := url.Parse(session.RedirectURL)
	if err != nil {
		t.Fatal(err)
	}
	data := checkoutURL.Query().Get("data")
	if checkoutURL.Query().Get("signature") != adapter.signature(data) {
		t.Fatal("checkout signature does not match payload")
	}
	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]string
	if err := json.Unmarshal(decoded, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["amount"] != "123.45" || payload["currency"] != "EUR" || payload["order_id"] != orderID.String() || payload["server_url"] != "https://api.example.test/webhooks/liqpay" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestVerifyWebhookValidatesSignatureAndMapsPaidEvent(t *testing.T) {
	adapter := newAdapter(t, Config{})
	orderID := uuid.New()
	data := callbackData(t, map[string]any{"order_id": orderID.String(), "payment_id": 42, "status": "success", "amount": 123.45, "currency": "EUR"})
	event, err := adapter.VerifyWebhook(context.Background(), paymentsDomain.WebhookRequest{Payload: []byte(url.Values{"data": {data}, "signature": {adapter.signature(data)}}.Encode())})
	if err != nil {
		t.Fatalf("VerifyWebhook() error = %v", err)
	}
	if event.EventID != "42:paid" || event.OrderID != orderID || event.ProviderReference != "42" || event.Status != "paid" || event.Amount.Amount != 12345 || event.Amount.Currency != "EUR" || event.OccurredAt.IsZero() {
		t.Fatalf("event = %+v", event)
	}

	_, err = adapter.VerifyWebhook(context.Background(), paymentsDomain.WebhookRequest{Payload: []byte(url.Values{"data": {data}, "signature": {"invalid"}}.Encode())})
	if !errors.Is(err, paymentsDomain.ErrInvalidWebhookSignature) {
		t.Fatalf("VerifyWebhook() error = %v, want ErrInvalidWebhookSignature", err)
	}
}

func TestRefundCallsProviderAPIWithSignedNeutralRequest(t *testing.T) {
	var received url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		received = r.PostForm
		_, _ = w.Write([]byte(`{"result":"ok"}`))
	}))
	defer server.Close()
	adapter := newAdapter(t, Config{APIURL: server.URL, HTTPClient: server.Client()})
	amount, _ := money.New(500, "EUR")
	if err := adapter.Refund(context.Background(), paymentsDomain.RefundRequest{PaymentReference: "order-1", Amount: amount, IdempotencyKey: "refund-1"}); err != nil {
		t.Fatalf("Refund() error = %v", err)
	}
	if received.Get("signature") != adapter.signature(received.Get("data")) {
		t.Fatal("refund signature does not match payload")
	}
	decoded, err := base64.StdEncoding.DecodeString(received.Get("data"))
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]string
	if err := json.Unmarshal(decoded, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["action"] != "refund" || payload["amount"] != "5.00" || payload["currency"] != "EUR" || payload["order_id"] != "order-1" {
		t.Fatalf("refund payload = %#v", payload)
	}
}

func newAdapter(t *testing.T, override Config) *Adapter {
	t.Helper()
	config := Config{PublicKey: "public", PrivateKey: "private", CallbackURL: "https://api.example.test/webhooks/liqpay", PriceScale: 2}
	if override.APIURL != "" {
		config.APIURL = override.APIURL
	}
	if override.HTTPClient != nil {
		config.HTTPClient = override.HTTPClient
	}
	adapter, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	return adapter
}

func callbackData(t *testing.T, payload map[string]any) string {
	t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(encoded)
}
