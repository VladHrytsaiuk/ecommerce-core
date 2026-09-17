package monobank

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
	paymentsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/payments/domain"
)

func TestVerifyWebhookValidatesECDSASignatureAndMapsStatus(t *testing.T) {
	adapter, privateKey := newAdapter(t, Config{})
	orderID := uuid.New()
	modifiedAt := time.Date(2026, time.August, 27, 12, 0, 0, 0, time.UTC)
	payload, err := json.Marshal(map[string]any{"invoiceId": "invoice-1", "status": "success", "amount": 12345, "ccy": 980, "reference": orderID.String(), "modifiedDate": modifiedAt.Format(time.RFC3339)})
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(payload)
	signature, err := ecdsa.SignASN1(rand.Reader, privateKey, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	event, err := adapter.VerifyWebhook(context.Background(), paymentsDomain.WebhookRequest{Headers: map[string]string{"X-Sign": base64.StdEncoding.EncodeToString(signature)}, Payload: payload})
	if err != nil {
		t.Fatalf("VerifyWebhook() error = %v", err)
	}
	if event.OrderID != orderID || event.ProviderReference != "invoice-1" || event.Status != "paid" || event.Amount.Amount() != 12345 || event.Amount.Currency() != "UAH" || !event.OccurredAt.Equal(modifiedAt) {
		t.Fatalf("event = %+v", event)
	}

	_, err = adapter.VerifyWebhook(context.Background(), paymentsDomain.WebhookRequest{Headers: map[string]string{"X-Sign": base64.StdEncoding.EncodeToString([]byte("invalid"))}, Payload: payload})
	if !errors.Is(err, paymentsDomain.ErrInvalidWebhookSignature) {
		t.Fatalf("VerifyWebhook() error = %v, want invalid signature", err)
	}
}

func TestCreateCheckoutAndRefundUseMonobankInvoiceAPI(t *testing.T) {
	var calls int
	orderID := uuid.New()
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("X-Token") != "token" {
			t.Fatalf("X-Token = %q", request.Header.Get("X-Token"))
		}
		calls++
		switch request.URL.Path {
		case "/api/merchant/invoice/create":
			var payload invoiceCreateRequest
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload.Amount != 12345 || payload.Ccy != monobankCurrencyUAH || payload.MerchantPaymInfo.Reference != orderID.String() || payload.WebhookURL != "https://api.example.test/api/webhooks/payments/monobank" {
				t.Fatalf("invoice payload = %+v", payload)
			}
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"invoiceId":"invoice-1","pageUrl":"https://pay.example/invoice-1"}`))
		case "/api/merchant/invoice/cancel":
			var payload invoiceCancelRequest
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload.InvoiceID != "invoice-1" || payload.ExternalReference != "refund-1" || payload.Amount != 12345 {
				t.Fatalf("cancel payload = %+v", payload)
			}
			writer.WriteHeader(http.StatusOK)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	adapter, _ := newAdapter(t, Config{APIURL: server.URL, HTTPClient: server.Client()})
	amount := mustMoney(12345, "UAH")
	session, err := adapter.CreateCheckout(context.Background(), paymentsDomain.CheckoutPayment{OrderID: orderID, IdempotencyKey: "checkout-1", Amount: amount})
	if err != nil || session.ProviderReference != "invoice-1" || session.RedirectURL != "https://pay.example/invoice-1" {
		t.Fatalf("CreateCheckout() = %+v, %v", session, err)
	}
	if err := adapter.Refund(context.Background(), paymentsDomain.RefundRequest{PaymentReference: "invoice-1", Amount: amount, IdempotencyKey: "refund-1"}); err != nil {
		t.Fatalf("Refund() error = %v", err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}

func newAdapter(t *testing.T, override Config) (*Adapter, *ecdsa.PrivateKey) {
	t.Helper()
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	encodedKey := base64.StdEncoding.EncodeToString(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
	config := Config{Token: "token", WebhookPublicKey: encodedKey, WebhookURL: "https://api.example.test/api/webhooks/payments/monobank"}
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
	return adapter, privateKey
}

func mustMoney(amount int64, currencyCode string) money.Money {
	value, err := money.NewMoney(amount, currencyCode)
	if err != nil {
		panic(err)
	}
	return value
}
