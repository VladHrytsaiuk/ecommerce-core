// Package liqpay translates the LiqPay protocol to the provider-neutral
// payments domain port. It never changes orders, taxes, or reservations.
package liqpay

import (
	"bytes"
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
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
	code                = "liqpay"
	defaultCheckoutURL  = "https://www.liqpay.ua/api/3/checkout"
	defaultAPIURL       = "https://www.liqpay.ua/api/request"
	maxResponseBodySize = 1 << 20
)

var (
	ErrInvalidSignature = errors.New("invalid liqpay signature")
	ErrInvalidPayload   = errors.New("invalid liqpay payload")
)

type Config struct {
	PublicKey   string
	PrivateKey  string
	CallbackURL string
	PriceScale  int
	CheckoutURL string
	APIURL      string
	HTTPClient  *http.Client
}

type Adapter struct {
	publicKey   string
	privateKey  string
	callbackURL string
	priceScale  int
	checkoutURL string
	apiURL      string
	httpClient  *http.Client
}

func New(config Config) (*Adapter, error) {
	config.PublicKey = strings.TrimSpace(config.PublicKey)
	config.PrivateKey = strings.TrimSpace(config.PrivateKey)
	config.CallbackURL = strings.TrimSpace(config.CallbackURL)
	if config.PublicKey == "" || config.PrivateKey == "" || config.CallbackURL == "" {
		return nil, fmt.Errorf("liqpay public key, private key and callback URL are required")
	}
	if _, err := url.ParseRequestURI(config.CallbackURL); err != nil {
		return nil, fmt.Errorf("invalid liqpay callback URL: %w", err)
	}
	if config.PriceScale < 0 || config.PriceScale > 6 {
		return nil, fmt.Errorf("invalid liqpay price scale %d", config.PriceScale)
	}
	if strings.TrimSpace(config.CheckoutURL) == "" {
		config.CheckoutURL = defaultCheckoutURL
	}
	if strings.TrimSpace(config.APIURL) == "" {
		config.APIURL = defaultAPIURL
	}
	if config.HTTPClient == nil {
		config.HTTPClient = http.DefaultClient
	}
	return &Adapter{publicKey: config.PublicKey, privateKey: config.PrivateKey, callbackURL: config.CallbackURL, priceScale: config.PriceScale, checkoutURL: config.CheckoutURL, apiURL: config.APIURL, httpClient: config.HTTPClient}, nil
}

func (*Adapter) Code() string { return code }

func (a *Adapter) CreateCheckout(_ context.Context, payment paymentsDomain.CheckoutPayment) (paymentsDomain.PaymentSession, error) {
	if payment.OrderID == uuid.Nil || strings.TrimSpace(payment.IdempotencyKey) == "" {
		return paymentsDomain.PaymentSession{}, fmt.Errorf("liqpay checkout requires order and idempotency key")
	}
	if _, err := money.New(payment.Amount.Amount, payment.Amount.Currency); err != nil {
		return paymentsDomain.PaymentSession{}, fmt.Errorf("invalid checkout amount: %w", err)
	}
	params := map[string]string{
		"public_key":  a.publicKey,
		"version":     "3",
		"action":      "pay",
		"amount":      formatAmount(payment.Amount.Amount, a.priceScale),
		"currency":    payment.Amount.Currency,
		"description": "Order " + payment.OrderID.String(),
		"order_id":    payment.OrderID.String(),
		"server_url":  a.callbackURL,
	}
	if returnURL := strings.TrimSpace(payment.ReturnURL); returnURL != "" {
		params["result_url"] = returnURL
	}
	data, err := encodePayload(params)
	if err != nil {
		return paymentsDomain.PaymentSession{}, err
	}
	checkoutURL, err := url.Parse(a.checkoutURL)
	if err != nil {
		return paymentsDomain.PaymentSession{}, fmt.Errorf("invalid liqpay checkout URL: %w", err)
	}
	query := checkoutURL.Query()
	query.Set("data", data)
	query.Set("signature", a.signature(data))
	checkoutURL.RawQuery = query.Encode()
	return paymentsDomain.PaymentSession{ProviderReference: payment.OrderID.String(), RedirectURL: checkoutURL.String()}, nil
}

func (a *Adapter) VerifyWebhook(_ context.Context, request paymentsDomain.WebhookRequest) (paymentsDomain.PaymentEvent, error) {
	form, err := url.ParseQuery(string(request.Payload))
	if err != nil {
		return paymentsDomain.PaymentEvent{}, fmt.Errorf("%w: parse form: %v", ErrInvalidPayload, err)
	}
	data, signature := form.Get("data"), form.Get("signature")
	if data == "" || signature == "" {
		return paymentsDomain.PaymentEvent{}, fmt.Errorf("%w: data and signature are required", ErrInvalidPayload)
	}
	if subtle.ConstantTimeCompare([]byte(a.signature(data)), []byte(signature)) != 1 {
		return paymentsDomain.PaymentEvent{}, fmt.Errorf("%w: %w", paymentsDomain.ErrInvalidWebhookSignature, ErrInvalidSignature)
	}
	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return paymentsDomain.PaymentEvent{}, fmt.Errorf("%w: decode data: %v", ErrInvalidPayload, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(decoded))
	decoder.UseNumber()
	var payload webhookPayload
	if err := decoder.Decode(&payload); err != nil {
		return paymentsDomain.PaymentEvent{}, fmt.Errorf("%w: decode JSON: %v", ErrInvalidPayload, err)
	}
	orderID, err := uuid.Parse(payload.OrderID)
	if err != nil {
		return paymentsDomain.PaymentEvent{}, fmt.Errorf("%w: order_id: %v", ErrInvalidPayload, err)
	}
	amount, err := parseAmount(payload.Amount.String(), a.priceScale, payload.Currency)
	if err != nil {
		return paymentsDomain.PaymentEvent{}, fmt.Errorf("%w: amount: %v", ErrInvalidPayload, err)
	}
	providerReference := strings.TrimSpace(payload.PaymentID.String())
	if providerReference == "" {
		hash := sha256.Sum256([]byte(data))
		providerReference = fmt.Sprintf("payload-%x", hash[:])
	}
	status := mapStatus(payload.Status)
	// A provider can report one payment first as pending and later as paid. The
	// status is part of the durable event identity, while payment_id remains the
	// provider reference shared by those events.
	return paymentsDomain.PaymentEvent{EventID: providerReference + ":" + status, OrderID: orderID, ProviderReference: providerReference, Status: status, Amount: amount, OccurredAt: time.Now().UTC()}, nil
}

func (a *Adapter) Refund(ctx context.Context, request paymentsDomain.RefundRequest) error {
	if strings.TrimSpace(request.PaymentReference) == "" || strings.TrimSpace(request.IdempotencyKey) == "" {
		return fmt.Errorf("liqpay refund requires payment reference and idempotency key")
	}
	if _, err := money.New(request.Amount.Amount, request.Amount.Currency); err != nil {
		return fmt.Errorf("invalid refund amount: %w", err)
	}
	data, err := encodePayload(map[string]string{
		"public_key": a.publicKey,
		"version":    "3",
		"action":     "refund",
		"order_id":   request.PaymentReference,
		"amount":     formatAmount(request.Amount.Amount, a.priceScale),
		"currency":   request.Amount.Currency,
		"language":   "en",
	})
	if err != nil {
		return err
	}
	body := url.Values{"data": {data}, "signature": {a.signature(data)}}.Encode()
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, a.apiURL, strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("create liqpay refund request: %w", err)
	}
	httpRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := a.httpClient.Do(httpRequest)
	if err != nil {
		return fmt.Errorf("send liqpay refund request: %w", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBodySize))
	if err != nil {
		return fmt.Errorf("read liqpay refund response: %w", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("liqpay refund response status %d", response.StatusCode)
	}
	var result struct {
		Result string `json:"result"`
	}
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return fmt.Errorf("decode liqpay refund response: %w", err)
	}
	if result.Result != "ok" {
		return fmt.Errorf("liqpay refund rejected")
	}
	return nil
}

type webhookPayload struct {
	OrderID   string      `json:"order_id"`
	PaymentID json.Number `json:"payment_id"`
	Status    string      `json:"status"`
	Amount    json.Number `json:"amount"`
	Currency  string      `json:"currency"`
}

func (a *Adapter) signature(data string) string {
	hash := sha1.Sum([]byte(a.privateKey + data + a.privateKey))
	return base64.StdEncoding.EncodeToString(hash[:])
}

func encodePayload(payload map[string]string) (string, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode liqpay payload: %w", err)
	}
	return base64.StdEncoding.EncodeToString(encoded), nil
}

func formatAmount(amount int64, scale int) string {
	if scale == 0 {
		return strconv.FormatInt(amount, 10)
	}
	base := int64(1)
	for range scale {
		base *= 10
	}
	return fmt.Sprintf("%d.%0*d", amount/base, scale, amount%base)
}

func parseAmount(value string, scale int, currency string) (money.Money, error) {
	parts := strings.Split(strings.TrimSpace(value), ".")
	if len(parts) > 2 || parts[0] == "" || strings.HasPrefix(parts[0], "-") {
		return money.Money{}, fmt.Errorf("invalid decimal amount %q", value)
	}
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
	}
	if len(fraction) > scale {
		return money.Money{}, fmt.Errorf("amount has more than %d decimal places", scale)
	}
	for len(fraction) < scale {
		fraction += "0"
	}
	minor, err := strconv.ParseInt(parts[0]+fraction, 10, 64)
	if err != nil {
		return money.Money{}, err
	}
	return money.New(minor, currency)
}

func mapStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "success", "sandbox":
		return "paid"
	case "failure", "error", "reversed":
		return "failed"
	default:
		return "pending"
	}
}

var _ paymentsDomain.Gateway = (*Adapter)(nil)
