// Package redsys translates Redsys redirect/REST messages to PaymentGateway.
package redsys

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
	paymentsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/payments/domain"
)

const (
	code           = "redsys"
	version        = "HMAC_SHA512_V2"
	defaultGateway = "https://sis.redsys.es/sis/realizarPago"
	defaultREST    = "https://sis.redsys.es/sis/rest/trataPeticionREST"
)

type Config struct {
	MerchantCode string
	Terminal     string
	SecretKey    string
	CallbackURL  string
	Currency     string
	CurrencyCode string
	GatewayURL   string
	RESTURL      string
	HTTPClient   *http.Client
}

type Adapter struct {
	merchantCode string
	terminal     string
	secretKey    string
	callbackURL  string
	currency     string
	currencyCode string
	gatewayURL   string
	restURL      string
	httpClient   *http.Client
}

func New(config Config) (*Adapter, error) {
	fields := []*string{&config.MerchantCode, &config.Terminal, &config.SecretKey, &config.CallbackURL, &config.Currency, &config.CurrencyCode}
	for _, field := range fields {
		*field = strings.TrimSpace(*field)
	}
	if config.MerchantCode == "" || config.Terminal == "" || config.SecretKey == "" || config.CallbackURL == "" || config.Currency == "" || config.CurrencyCode == "" {
		return nil, fmt.Errorf("redsys merchant code, terminal, secret key, callback URL, currency and currency code are required")
	}
	if _, err := url.ParseRequestURI(config.CallbackURL); err != nil {
		return nil, fmt.Errorf("invalid redsys callback URL: %w", err)
	}
	if len(config.CurrencyCode) > 4 || !isDigits(config.CurrencyCode) {
		return nil, fmt.Errorf("invalid redsys currency code")
	}
	if config.GatewayURL == "" {
		config.GatewayURL = defaultGateway
	}
	if config.RESTURL == "" {
		config.RESTURL = defaultREST
	}
	if config.HTTPClient == nil {
		config.HTTPClient = http.DefaultClient
	}
	return &Adapter{merchantCode: config.MerchantCode, terminal: config.Terminal, secretKey: config.SecretKey, callbackURL: config.CallbackURL, currency: strings.ToUpper(config.Currency), currencyCode: config.CurrencyCode, gatewayURL: config.GatewayURL, restURL: config.RESTURL, httpClient: config.HTTPClient}, nil
}

func (*Adapter) Code() string { return code }

func (a *Adapter) CreateCheckout(_ context.Context, payment paymentsDomain.CheckoutPayment) (paymentsDomain.PaymentSession, error) {
	if payment.OrderID == uuid.Nil || strings.TrimSpace(payment.IdempotencyKey) == "" || payment.Amount.Amount <= 0 {
		return paymentsDomain.PaymentSession{}, fmt.Errorf("redsys checkout requires order, amount and idempotency key")
	}
	if _, err := money.New(payment.Amount.Amount, payment.Amount.Currency); err != nil || payment.Amount.Currency != a.currency {
		return paymentsDomain.PaymentSession{}, fmt.Errorf("redsys checkout currency must be %s", a.currency)
	}
	orderRef := orderReference(payment.OrderID)
	params := map[string]string{
		"DS_MERCHANT_ORDER": orderRef, "DS_MERCHANT_MERCHANTCODE": a.merchantCode, "DS_MERCHANT_TERMINAL": a.terminal,
		"DS_MERCHANT_CURRENCY": a.currencyCode, "DS_MERCHANT_TRANSACTIONTYPE": "0", "DS_MERCHANT_AMOUNT": strconv.FormatInt(payment.Amount.Amount, 10),
		"DS_MERCHANT_MERCHANTURL": a.callbackURL, "DS_MERCHANT_MERCHANTDATA": payment.OrderID.String(),
	}
	if value := strings.TrimSpace(payment.ReturnURL); value != "" {
		params["DS_MERCHANT_URLOK"] = value
	}
	if value := strings.TrimSpace(payment.CancelURL); value != "" {
		params["DS_MERCHANT_URLKO"] = value
	}
	encoded, signature, err := a.sign(params, orderRef)
	if err != nil {
		return paymentsDomain.PaymentSession{}, err
	}
	return paymentsDomain.PaymentSession{ProviderReference: orderRef, RedirectURL: a.gatewayURL, FormFields: map[string]string{"Ds_SignatureVersion": version, "Ds_MerchantParameters": encoded, "Ds_Signature": signature}}, nil
}

func (a *Adapter) VerifyWebhook(_ context.Context, request paymentsDomain.WebhookRequest) (paymentsDomain.PaymentEvent, error) {
	form, err := url.ParseQuery(string(request.Payload))
	if err != nil {
		return paymentsDomain.PaymentEvent{}, err
	}
	encoded, received := form.Get("Ds_MerchantParameters"), form.Get("Ds_Signature")
	params, err := decode(encoded)
	if err != nil {
		return paymentsDomain.PaymentEvent{}, fmt.Errorf("decode redsys webhook: %w", err)
	}
	orderRef := value(params, "DS_ORDER")
	if orderRef == "" || !a.verify(encoded, orderRef, received) {
		return paymentsDomain.PaymentEvent{}, paymentsDomain.ErrInvalidWebhookSignature
	}
	orderID, err := uuid.Parse(value(params, "DS_MERCHANTDATA"))
	if err != nil {
		return paymentsDomain.PaymentEvent{}, fmt.Errorf("redsys merchant data: %w", err)
	}
	if value(params, "DS_CURRENCY") != a.currencyCode {
		return paymentsDomain.PaymentEvent{}, fmt.Errorf("redsys webhook currency does not match store configuration")
	}
	amountValue, err := strconv.ParseInt(value(params, "DS_AMOUNT"), 10, 64)
	if err != nil {
		return paymentsDomain.PaymentEvent{}, fmt.Errorf("redsys webhook amount: %w", err)
	}
	amount, err := money.New(amountValue, a.currency)
	if err != nil {
		return paymentsDomain.PaymentEvent{}, err
	}
	// The generic port has ISO currency. The ISO code is carried in merchant
	// data so an adapter configuration cannot silently relabel a payment.
	if raw := value(params, "DS_MERCHANTDATA"); raw == "" {
		return paymentsDomain.PaymentEvent{}, fmt.Errorf("redsys merchant data is required")
	}
	status := "failed"
	response, _ := strconv.Atoi(value(params, "DS_RESPONSE"))
	if response >= 0 && response < 100 {
		status = "paid"
	}
	eventID := orderRef + ":" + value(params, "DS_RESPONSE") + ":" + value(params, "DS_AUTHORISATIONCODE")
	return paymentsDomain.PaymentEvent{EventID: eventID, OrderID: orderID, ProviderReference: orderRef, Status: status, Amount: amount}, nil
}

func (a *Adapter) Refund(ctx context.Context, request paymentsDomain.RefundRequest) error {
	if strings.TrimSpace(request.PaymentReference) == "" || strings.TrimSpace(request.IdempotencyKey) == "" || request.Amount.Amount <= 0 || request.Amount.Currency != a.currency {
		return fmt.Errorf("redsys refund requires payment reference, configured-currency amount and idempotency key")
	}
	params := map[string]string{"DS_MERCHANT_ORDER": request.PaymentReference, "DS_MERCHANT_MERCHANTCODE": a.merchantCode, "DS_MERCHANT_TERMINAL": a.terminal, "DS_MERCHANT_CURRENCY": a.currencyCode, "DS_MERCHANT_TRANSACTIONTYPE": "3", "DS_MERCHANT_AMOUNT": strconv.FormatInt(request.Amount.Amount, 10)}
	encoded, signature, err := a.sign(params, request.PaymentReference)
	if err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]string{"Ds_SignatureVersion": version, "Ds_MerchantParameters": encoded, "Ds_Signature": signature})
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, a.restURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := a.httpClient.Do(httpRequest)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("redsys refund response status %d", response.StatusCode)
	}
	responseBytes, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return err
	}
	var responseBody map[string]string
	if err := json.Unmarshal(responseBytes, &responseBody); err != nil {
		return fmt.Errorf("decode redsys refund response: %w", err)
	}
	responseParams, err := decode(responseBody["Ds_MerchantParameters"])
	if err != nil || !a.verify(responseBody["Ds_MerchantParameters"], request.PaymentReference, responseBody["Ds_Signature"]) {
		return fmt.Errorf("invalid redsys refund response signature")
	}
	responseCode, err := strconv.Atoi(value(responseParams, "DS_RESPONSE"))
	if err != nil || responseCode < 0 || responseCode >= 100 {
		return fmt.Errorf("redsys refund was rejected")
	}
	return nil
}

func (a *Adapter) sign(params map[string]string, order string) (string, string, error) {
	raw, err := json.Marshal(params)
	if err != nil {
		return "", "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(raw)
	signature, err := a.signature(encoded, order)
	return encoded, signature, err
}
func (a *Adapter) verify(encoded, order, received string) bool {
	expected, err := a.signature(encoded, order)
	return err == nil && subtle.ConstantTimeCompare([]byte(expected), []byte(received)) == 1
}
func (a *Adapter) signature(encoded, order string) (string, error) {
	key, err := diversify(a.secretKey, order)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha512.New, key)
	_, _ = mac.Write([]byte(encoded))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}
func diversify(secret, order string) ([]byte, error) {
	secret = secret + strings.Repeat("0", 16)
	block, err := aes.NewCipher([]byte(secret[:16]))
	if err != nil {
		return nil, err
	}
	plain := pkcs7([]byte(order), aes.BlockSize)
	encrypted := make([]byte, len(plain))
	cipher.NewCBCEncrypter(block, make([]byte, aes.BlockSize)).CryptBlocks(encrypted, plain)
	result := make([]byte, base64.StdEncoding.EncodedLen(len(encrypted)))
	base64.StdEncoding.Encode(result, encrypted)
	return result, nil
}
func pkcs7(value []byte, blockSize int) []byte {
	padding := blockSize - len(value)%blockSize
	return append(value, bytes.Repeat([]byte{byte(padding)}, padding)...)
}
func decode(encoded string) (map[string]any, error) {
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		raw, err = base64.URLEncoding.DecodeString(encoded)
	}
	if err != nil {
		return nil, err
	}
	var result map[string]any
	err = json.Unmarshal(raw, &result)
	return result, err
}
func value(params map[string]any, key string) string {
	for candidate, result := range params {
		if strings.EqualFold(candidate, key) {
			return fmt.Sprint(result)
		}
	}
	return ""
}
func orderReference(orderID uuid.UUID) string {
	sum := sha256.Sum256(orderID[:])
	number := binary.BigEndian.Uint64(sum[:8]) % 4738381338321616896
	value := strings.ToUpper(strconv.FormatUint(number, 36))
	return strings.Repeat("0", 12-len(value)) + value
}
func isDigits(value string) bool {
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

var _ paymentsDomain.Gateway = (*Adapter)(nil)
