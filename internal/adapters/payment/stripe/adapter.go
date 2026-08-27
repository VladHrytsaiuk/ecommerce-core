// Package stripe translates Stripe's HTTP API to the provider-neutral payment
// Gateway port. It owns neither checkout policy nor order transitions.
package stripe

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
	paymentsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/payments/domain"
)

const (
	code                = "stripe"
	defaultAPIURL       = "https://api.stripe.com"
	maxResponseBodySize = 1 << 20
	defaultTolerance    = 5 * time.Minute
)

type Config struct {
	SecretKey        string
	WebhookSecret    string
	APIURL           string
	HTTPClient       *http.Client
	WebhookTolerance time.Duration
}

type Adapter struct {
	secretKey        string
	webhookSecret    string
	apiURL           string
	httpClient       *http.Client
	webhookTolerance time.Duration
	now              func() time.Time
}

func New(config Config) (*Adapter, error) {
	config.SecretKey = strings.TrimSpace(config.SecretKey)
	config.WebhookSecret = strings.TrimSpace(config.WebhookSecret)
	if config.SecretKey == "" || config.WebhookSecret == "" {
		return nil, fmt.Errorf("stripe secret key and webhook secret are required")
	}
	if strings.TrimSpace(config.APIURL) == "" {
		config.APIURL = defaultAPIURL
	}
	if _, err := url.ParseRequestURI(config.APIURL); err != nil {
		return nil, fmt.Errorf("invalid stripe API URL: %w", err)
	}
	if config.HTTPClient == nil {
		config.HTTPClient = http.DefaultClient
	}
	if config.WebhookTolerance <= 0 {
		config.WebhookTolerance = defaultTolerance
	}
	return &Adapter{secretKey: config.SecretKey, webhookSecret: config.WebhookSecret, apiURL: strings.TrimRight(config.APIURL, "/"), httpClient: config.HTTPClient, webhookTolerance: config.WebhookTolerance, now: time.Now}, nil
}

func (*Adapter) Code() string { return code }

// CreateCheckout creates a PaymentIntent. The returned client secret belongs
// only to the buyer's browser; the caller must not log or persist it.
func (a *Adapter) CreateCheckout(ctx context.Context, payment paymentsDomain.CheckoutPayment) (paymentsDomain.PaymentSession, error) {
	if payment.OrderID == uuid.Nil || strings.TrimSpace(payment.IdempotencyKey) == "" {
		return paymentsDomain.PaymentSession{}, fmt.Errorf("stripe checkout requires order and idempotency key")
	}
	if payment.Amount.Amount() <= 0 {
		return paymentsDomain.PaymentSession{}, fmt.Errorf("stripe checkout amount must be positive")
	}
	if err := payment.Amount.Validate(); err != nil {
		return paymentsDomain.PaymentSession{}, fmt.Errorf("invalid stripe checkout amount: %w", err)
	}

	form := url.Values{
		"amount":                             {strconv.FormatInt(payment.Amount.Amount(), 10)},
		"currency":                           {strings.ToLower(payment.Amount.Currency())},
		"automatic_payment_methods[enabled]": {"true"},
		"metadata[order_id]":                 {payment.OrderID.String()},
		"metadata[checkout_id]":              {payment.IdempotencyKey},
		"description":                        {"Order " + payment.OrderID.String()},
	}
	var response struct {
		ID           string `json:"id"`
		ClientSecret string `json:"client_secret"`
	}
	if err := a.postForm(ctx, "/v1/payment_intents", form, payment.IdempotencyKey, &response); err != nil {
		return paymentsDomain.PaymentSession{}, err
	}
	if strings.TrimSpace(response.ID) == "" || strings.TrimSpace(response.ClientSecret) == "" {
		return paymentsDomain.PaymentSession{}, fmt.Errorf("stripe payment intent response is incomplete")
	}
	return paymentsDomain.PaymentSession{ProviderReference: response.ID, ClientSecret: response.ClientSecret}, nil
}

func (a *Adapter) VerifyWebhook(_ context.Context, request paymentsDomain.WebhookRequest) (paymentsDomain.PaymentEvent, error) {
	signature := header(request.Headers, "Stripe-Signature")
	if !a.validSignature(signature, request.Payload) {
		return paymentsDomain.PaymentEvent{}, paymentsDomain.ErrInvalidWebhookSignature
	}
	var event stripeEvent
	if err := json.Unmarshal(request.Payload, &event); err != nil {
		return paymentsDomain.PaymentEvent{}, fmt.Errorf("decode stripe webhook: %w", err)
	}
	orderID, err := uuid.Parse(event.Data.Object.Metadata.OrderID)
	if err != nil {
		return paymentsDomain.PaymentEvent{}, fmt.Errorf("stripe webhook order metadata: %w", err)
	}
	amountValue := event.Data.Object.Amount
	if event.Type == "payment_intent.succeeded" {
		amountValue = event.Data.Object.AmountReceived
	}
	amount, err := money.NewMoney(amountValue, event.Data.Object.Currency)
	if err != nil {
		return paymentsDomain.PaymentEvent{}, fmt.Errorf("stripe webhook amount: %w", err)
	}
	status := "pending"
	switch event.Type {
	case "payment_intent.succeeded":
		status = "paid"
	case "payment_intent.payment_failed", "payment_intent.canceled":
		status = "failed"
	case "payment_intent.processing", "payment_intent.requires_action":
		status = "pending"
	}
	occurredAt := time.Unix(event.Created, 0).UTC()
	if event.Created <= 0 {
		occurredAt = a.now().UTC()
	}
	if strings.TrimSpace(event.ID) == "" || strings.TrimSpace(event.Data.Object.ID) == "" {
		return paymentsDomain.PaymentEvent{}, fmt.Errorf("stripe webhook event identity is required")
	}
	return paymentsDomain.PaymentEvent{EventID: event.ID, Provider: code, OrderID: orderID, ProviderReference: event.Data.Object.ID, Status: status, Amount: amount, OccurredAt: occurredAt}, nil
}

func (a *Adapter) Refund(ctx context.Context, request paymentsDomain.RefundRequest) error {
	if strings.TrimSpace(request.PaymentReference) == "" || strings.TrimSpace(request.IdempotencyKey) == "" || request.Amount.Amount() <= 0 {
		return fmt.Errorf("stripe refund requires payment reference, amount and idempotency key")
	}
	if err := request.Amount.Validate(); err != nil {
		return fmt.Errorf("invalid stripe refund amount: %w", err)
	}
	var response struct {
		ID string `json:"id"`
	}
	if err := a.postForm(ctx, "/v1/refunds", url.Values{"payment_intent": {request.PaymentReference}, "amount": {strconv.FormatInt(request.Amount.Amount(), 10)}}, request.IdempotencyKey, &response); err != nil {
		return err
	}
	if strings.TrimSpace(response.ID) == "" {
		return fmt.Errorf("stripe refund response is incomplete")
	}
	return nil
}

func (a *Adapter) postForm(ctx context.Context, path string, form url.Values, idempotencyKey string, target any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, a.apiURL+path, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("create stripe request: %w", err)
	}
	request.SetBasicAuth(a.secretKey, "")
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Idempotency-Key", idempotencyKey)
	response, err := a.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("send stripe request: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBodySize))
	if err != nil {
		return fmt.Errorf("read stripe response: %w", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("stripe response status %d", response.StatusCode)
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("decode stripe response: %w", err)
	}
	return nil
}

func (a *Adapter) validSignature(value string, payload []byte) bool {
	parts := strings.Split(value, ",")
	var timestamp string
	var signatures []string
	for _, part := range parts {
		key, value, found := strings.Cut(strings.TrimSpace(part), "=")
		if !found {
			continue
		}
		switch key {
		case "t":
			timestamp = value
		case "v1":
			signatures = append(signatures, value)
		}
	}
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || len(signatures) == 0 {
		return false
	}
	occurredAt := time.Unix(seconds, 0)
	if elapsed := a.now().Sub(occurredAt); elapsed > a.webhookTolerance || elapsed < -a.webhookTolerance {
		return false
	}
	mac := hmac.New(sha256.New, []byte(a.webhookSecret))
	_, _ = mac.Write([]byte(timestamp + "."))
	_, _ = mac.Write(payload)
	expected := mac.Sum(nil)
	for _, signature := range signatures {
		actual, err := hex.DecodeString(signature)
		if err == nil && subtle.ConstantTimeCompare(expected, actual) == 1 {
			return true
		}
	}
	return false
}

func header(headers map[string]string, name string) string {
	for key, value := range headers {
		if strings.EqualFold(key, name) {
			return value
		}
	}
	return ""
}

type stripeEvent struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Created int64  `json:"created"`
	Data    struct {
		Object struct {
			ID             string `json:"id"`
			Amount         int64  `json:"amount"`
			AmountReceived int64  `json:"amount_received"`
			Currency       string `json:"currency"`
			Metadata       struct {
				OrderID string `json:"order_id"`
			} `json:"metadata"`
		} `json:"object"`
	} `json:"data"`
}

var _ paymentsDomain.Gateway = (*Adapter)(nil)
