// Package monobank translates Monobank acquiring HTTP messages to the
// provider-neutral payments Gateway. It owns protocol authentication only;
// the OrderWorkflow atomically validates payment snapshots and changes orders.
package monobank

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
	paymentsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/payments/domain"
)

const (
	code                = "monobank"
	defaultAPIURL       = "https://api.monobank.ua"
	monobankCurrencyUAH = 980
	maxResponseBodySize = 1 << 20
)

var ErrInvalidPayload = errors.New("invalid monobank payload")

// Config holds only provider protocol configuration. WebhookPublicKey is the
// base64-encoded PEM public key returned by Monobank's merchant pubkey API.
type Config struct {
	Token            string
	WebhookPublicKey string
	APIURL           string
	WebhookURL       string
	HTTPClient       *http.Client
}

type Adapter struct {
	token      string
	publicKey  *ecdsa.PublicKey
	apiURL     string
	webhookURL string
	httpClient *http.Client
	now        func() time.Time
}

func New(config Config) (*Adapter, error) {
	config.Token = strings.TrimSpace(config.Token)
	config.WebhookPublicKey = strings.TrimSpace(config.WebhookPublicKey)
	config.WebhookURL = strings.TrimSpace(config.WebhookURL)
	if config.Token == "" || config.WebhookPublicKey == "" || config.WebhookURL == "" {
		return nil, fmt.Errorf("monobank token, webhook public key and webhook URL are required")
	}
	if strings.TrimSpace(config.APIURL) == "" {
		config.APIURL = defaultAPIURL
	}
	apiURL, err := parseHTTPSURL(config.APIURL)
	if err != nil {
		return nil, fmt.Errorf("invalid monobank API URL: %w", err)
	}
	if _, err := parseHTTPSURL(config.WebhookURL); err != nil {
		return nil, fmt.Errorf("invalid monobank webhook URL: %w", err)
	}
	publicKey, err := parsePublicKey(config.WebhookPublicKey)
	if err != nil {
		return nil, err
	}
	if config.HTTPClient == nil {
		config.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Adapter{token: config.Token, publicKey: publicKey, apiURL: strings.TrimRight(apiURL.String(), "/"), webhookURL: config.WebhookURL, httpClient: config.HTTPClient, now: time.Now}, nil
}

func (*Adapter) Code() string { return code }

func (a *Adapter) CreateCheckout(ctx context.Context, payment paymentsDomain.CheckoutPayment) (paymentsDomain.PaymentSession, error) {
	if payment.OrderID == uuid.Nil || strings.TrimSpace(payment.IdempotencyKey) == "" {
		return paymentsDomain.PaymentSession{}, fmt.Errorf("monobank checkout requires order and idempotency key")
	}
	if err := payment.Amount.Validate(); err != nil || payment.Amount.Amount() <= 0 || payment.Amount.Currency() != "UAH" {
		return paymentsDomain.PaymentSession{}, fmt.Errorf("monobank checkout requires a positive UAH amount")
	}

	payload := invoiceCreateRequest{
		Amount: payment.Amount.Amount(), Ccy: monobankCurrencyUAH,
		MerchantPaymInfo: merchantPaymentInfo{Reference: payment.OrderID.String(), Destination: "Order " + payment.OrderID.String()},
		WebhookURL:       a.webhookURL,
	}
	if returnURL := strings.TrimSpace(payment.ReturnURL); returnURL != "" {
		payload.RedirectURL = returnURL
	}
	var response invoiceCreateResponse
	if err := a.postJSON(ctx, "/api/merchant/invoice/create", payload, &response); err != nil {
		return paymentsDomain.PaymentSession{}, err
	}
	if strings.TrimSpace(response.InvoiceID) == "" || strings.TrimSpace(response.PageURL) == "" {
		return paymentsDomain.PaymentSession{}, fmt.Errorf("monobank invoice response is incomplete")
	}
	return paymentsDomain.PaymentSession{ProviderReference: response.InvoiceID, RedirectURL: response.PageURL}, nil
}

// VerifyWebhook first validates X-Sign over the untouched request bytes. JSON
// must not be decoded or re-marshaled before this point, since Monobank signs
// the original body using ECDSA/SHA-256.
func (a *Adapter) VerifyWebhook(_ context.Context, request paymentsDomain.WebhookRequest) (paymentsDomain.PaymentEvent, error) {
	signature, err := base64.StdEncoding.DecodeString(header(request.Headers, "X-Sign"))
	if err != nil || len(signature) == 0 {
		return paymentsDomain.PaymentEvent{}, paymentsDomain.ErrInvalidWebhookSignature
	}
	digest := sha256.Sum256(request.Payload)
	if !ecdsa.VerifyASN1(a.publicKey, digest[:], signature) {
		return paymentsDomain.PaymentEvent{}, paymentsDomain.ErrInvalidWebhookSignature
	}

	var payload webhookPayload
	// Monobank can add transport/fiscalisation fields to this signed payload.
	// Decode only the stable fields owned by the Gateway contract; rejecting an
	// otherwise verified callback merely because a provider added a field would
	// create a financial reconciliation gap.
	if err := json.Unmarshal(request.Payload, &payload); err != nil {
		return paymentsDomain.PaymentEvent{}, fmt.Errorf("%w: decode JSON", ErrInvalidPayload)
	}
	if payload.InvoiceID == "" || payload.Reference == "" || payload.Amount <= 0 || payload.Ccy != monobankCurrencyUAH {
		return paymentsDomain.PaymentEvent{}, fmt.Errorf("%w: required invoice fields are invalid", ErrInvalidPayload)
	}
	orderID, err := uuid.Parse(payload.Reference)
	if err != nil {
		return paymentsDomain.PaymentEvent{}, fmt.Errorf("%w: reference is not an order UUID", ErrInvalidPayload)
	}
	amount, err := money.NewMoney(payload.Amount, "UAH")
	if err != nil {
		return paymentsDomain.PaymentEvent{}, fmt.Errorf("%w: amount", ErrInvalidPayload)
	}
	status, err := mapStatus(payload.Status)
	if err != nil {
		return paymentsDomain.PaymentEvent{}, err
	}
	occurredAt := a.now().UTC()
	eventSuffix := ""
	if rawDate := strings.TrimSpace(payload.ModifiedDate); rawDate != "" {
		parsed, err := time.Parse(time.RFC3339, rawDate)
		if err != nil {
			return paymentsDomain.PaymentEvent{}, fmt.Errorf("%w: modifiedDate", ErrInvalidPayload)
		}
		occurredAt = parsed.UTC()
		eventSuffix = rawDate
	} else {
		// A signed raw-body hash keeps retries with a missing modifiedDate
		// idempotent without trusting a client-controlled generated identifier.
		eventSuffix = fmt.Sprintf("%x", sha256.Sum256(request.Payload))
	}
	return paymentsDomain.PaymentEvent{
		EventID: payload.InvoiceID + ":" + status + ":" + eventSuffix, Provider: code,
		OrderID: orderID, ProviderReference: payload.InvoiceID, Status: status, Amount: amount, OccurredAt: occurredAt,
	}, nil
}

// Refund invokes Monobank's invoice cancellation endpoint. The caller-provided
// idempotency key is an external reference recognised by Monobank.
func (a *Adapter) Refund(ctx context.Context, request paymentsDomain.RefundRequest) error {
	if strings.TrimSpace(request.PaymentReference) == "" || strings.TrimSpace(request.IdempotencyKey) == "" {
		return fmt.Errorf("monobank refund requires payment reference and idempotency key")
	}
	if err := request.Amount.Validate(); err != nil || request.Amount.Amount() <= 0 || request.Amount.Currency() != "UAH" {
		return fmt.Errorf("monobank refund requires a positive UAH amount")
	}
	return a.postJSON(ctx, "/api/merchant/invoice/cancel", invoiceCancelRequest{InvoiceID: request.PaymentReference, ExternalReference: request.IdempotencyKey, Amount: request.Amount.Amount()}, nil)
}

func (a *Adapter) postJSON(ctx context.Context, path string, payload any, target any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal monobank request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, a.apiURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create monobank request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Token", a.token)
	response, err := a.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("send monobank request: %w", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBodySize))
	if err != nil {
		return fmt.Errorf("read monobank response: %w", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("monobank request rejected with status %d", response.StatusCode)
	}
	if target != nil && len(responseBody) > 0 {
		if err := json.Unmarshal(responseBody, target); err != nil {
			return fmt.Errorf("decode monobank response: %w", err)
		}
	}
	return nil
}

func parsePublicKey(encoded string) (*ecdsa.PublicKey, error) {
	der, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode monobank webhook public key: %w", err)
	}
	block, rest := pem.Decode(der)
	if block == nil || len(rest) != 0 || block.Type != "PUBLIC KEY" {
		return nil, fmt.Errorf("monobank webhook public key must be a PEM public key")
	}
	value, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse monobank webhook public key: %w", err)
	}
	key, ok := value.(*ecdsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("monobank webhook public key must be ECDSA")
	}
	return key, nil
}

func parseHTTPSURL(raw string) (*url.URL, error) {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return nil, errors.New("URL must be absolute HTTPS")
	}
	return parsed, nil
}

func header(headers map[string]string, name string) string {
	for key, value := range headers {
		if strings.EqualFold(key, name) {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func mapStatus(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "success":
		return "paid", nil
	case "failure", "expired":
		return "failed", nil
	case "reversed":
		return "refunded", nil
	case "created", "processing", "hold":
		return "pending", nil
	default:
		return "", fmt.Errorf("%w: unsupported status", ErrInvalidPayload)
	}
}

type merchantPaymentInfo struct {
	Reference   string `json:"reference"`
	Destination string `json:"destination"`
}

type invoiceCreateRequest struct {
	Amount           int64               `json:"amount"`
	Ccy              int                 `json:"ccy"`
	MerchantPaymInfo merchantPaymentInfo `json:"merchantPaymInfo"`
	RedirectURL      string              `json:"redirectUrl,omitempty"`
	WebhookURL       string              `json:"webHookUrl"`
}

type invoiceCreateResponse struct {
	InvoiceID string `json:"invoiceId"`
	PageURL   string `json:"pageUrl"`
}

type invoiceCancelRequest struct {
	InvoiceID         string `json:"invoiceId"`
	ExternalReference string `json:"extRef"`
	Amount            int64  `json:"amount"`
}

type webhookPayload struct {
	InvoiceID    string `json:"invoiceId"`
	Status       string `json:"status"`
	Amount       int64  `json:"amount"`
	Ccy          int    `json:"ccy"`
	Reference    string `json:"reference"`
	ModifiedDate string `json:"modifiedDate"`
}

var _ paymentsDomain.Gateway = (*Adapter)(nil)
