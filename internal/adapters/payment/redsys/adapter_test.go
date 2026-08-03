package redsys

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
	paymentsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/payments/domain"
)

func TestCreateCheckoutBuildsSignedRedirectForm(t *testing.T) {
	adapter := newAdapter(t)
	amount, _ := money.New(249, "EUR")
	orderID := uuid.New()
	session, err := adapter.CreateCheckout(context.Background(), paymentsDomain.CheckoutPayment{OrderID: orderID, IdempotencyKey: "checkout-1", Amount: amount, ReturnURL: "https://store.test/ok", CancelURL: "https://store.test/ko"})
	if err != nil {
		t.Fatal(err)
	}
	params, err := decode(session.FormFields["Ds_MerchantParameters"])
	if err != nil {
		t.Fatal(err)
	}
	if session.RedirectURL != "https://gateway.test/pay" || session.ProviderReference != value(params, "DS_MERCHANT_ORDER") || value(params, "DS_MERCHANT_AMOUNT") != "249" || value(params, "DS_MERCHANT_MERCHANTDATA") != orderID.String() || !adapter.verify(session.FormFields["Ds_MerchantParameters"], session.ProviderReference, session.FormFields["Ds_Signature"]) {
		t.Fatalf("session=%+v params=%v", session, params)
	}
}

func TestVerifyWebhookValidatesSignatureAndMapsPaidEvent(t *testing.T) {
	adapter := newAdapter(t)
	orderID := uuid.New()
	orderRef := orderReference(orderID)
	encoded, signature, err := adapter.sign(map[string]string{"DS_ORDER": orderRef, "DS_MERCHANTDATA": orderID.String(), "DS_CURRENCY": "978", "DS_AMOUNT": "249", "DS_RESPONSE": "0000", "DS_AUTHORISATIONCODE": "123456"}, orderRef)
	if err != nil {
		t.Fatal(err)
	}
	event, err := adapter.VerifyWebhook(context.Background(), paymentsDomain.WebhookRequest{Payload: []byte(url.Values{"Ds_MerchantParameters": {encoded}, "Ds_Signature": {signature}}.Encode())})
	if err != nil {
		t.Fatal(err)
	}
	if event.OrderID != orderID || event.ProviderReference != orderRef || event.Status != "paid" || event.Amount.Amount != 249 || event.Amount.Currency != "EUR" {
		t.Fatalf("event=%+v", event)
	}
	_, err = adapter.VerifyWebhook(context.Background(), paymentsDomain.WebhookRequest{Payload: []byte(url.Values{"Ds_MerchantParameters": {encoded}, "Ds_Signature": {"bad"}}.Encode())})
	if err != paymentsDomain.ErrInvalidWebhookSignature {
		t.Fatalf("error=%v", err)
	}
}

func TestRefundPostsSignedRESTRequest(t *testing.T) {
	adapter := newAdapter(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]string
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		params, err := decode(request["Ds_MerchantParameters"])
		if err != nil {
			t.Fatal(err)
		}
		if value(params, "DS_MERCHANT_ORDER") != "0000ABCDEF12" || value(params, "DS_MERCHANT_TRANSACTIONTYPE") != "3" {
			t.Fatalf("params=%v", params)
		}
		responseParams, responseSignature, err := adapter.sign(map[string]string{"DS_ORDER": "0000ABCDEF12", "DS_RESPONSE": "0000"}, "0000ABCDEF12")
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(`{"Ds_MerchantParameters":"` + responseParams + `","Ds_Signature":"` + responseSignature + `"}`))
	}))
	defer server.Close()
	adapter.restURL = server.URL
	amount, _ := money.New(249, "EUR")
	if err := adapter.Refund(context.Background(), paymentsDomain.RefundRequest{PaymentReference: "0000ABCDEF12", Amount: amount, IdempotencyKey: "refund-1"}); err != nil {
		t.Fatal(err)
	}
}

func TestDiversifyMatchesRedsysV2PublishedExample(t *testing.T) {
	key, err := diversify("sq7HjrUOBfKmC576ILgskD5srU870gJ7", "1234567890")
	if err != nil {
		t.Fatal(err)
	}
	if string(key) != "RWt3/IPTzYRMXsQtkiGRKg==" {
		t.Fatalf("derived key = %q", key)
	}
}

func newAdapter(t *testing.T) *Adapter {
	t.Helper()
	adapter, err := New(Config{MerchantCode: "999008881", Terminal: "001", SecretKey: "sq7HjrUOBfKmC576ILgskD5srU870gJ7", CallbackURL: "https://api.test/webhooks/payments/redsys", Currency: "EUR", CurrencyCode: "978", GatewayURL: "https://gateway.test/pay"})
	if err != nil {
		t.Fatal(err)
	}
	return adapter
}

func TestOrderReferenceHasProviderSafeLength(t *testing.T) {
	reference := orderReference(uuid.New())
	if len(reference) != 12 {
		t.Fatalf("reference=%q", reference)
	}
	if _, err := strconv.ParseUint(reference, 36, 64); err != nil {
		t.Fatal(err)
	}
}
