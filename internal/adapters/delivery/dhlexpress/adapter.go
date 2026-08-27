// Package dhlexpress translates DHL Express MyDHL HTTP API calls to the
// provider-neutral delivery Carrier port. Checkout and order state changes
// intentionally remain outside this adapter.
package dhlexpress

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
	deliveryDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/delivery/domain"
)

const code = "dhlexpress"

// Config contains DHL Express account and origin data. Package dimensions are
// deployment policy, rather than a checkout concern: the current neutral port
// models product weight but not dimensions. Configure values that match the
// store's standard parcel; a future packaging module can replace this policy.
type Config struct {
	BaseURL       string
	Username      string
	Password      string
	AccountNumber string
	ProductCode   string
	SenderName    string
	SenderPhone   string
	SenderCountry string
	SenderPostal  string
	SenderCity    string
	SenderLine1   string
	PackageLength float64
	PackageWidth  float64
	PackageHeight float64
	PriceScale    int
	HTTPClient    *http.Client
}

type Adapter struct{ config Config }

func New(config Config) (*Adapter, error) {
	config.BaseURL = strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	config.Username = strings.TrimSpace(config.Username)
	config.Password = strings.TrimSpace(config.Password)
	config.AccountNumber = strings.TrimSpace(config.AccountNumber)
	config.ProductCode = strings.TrimSpace(config.ProductCode)
	config.SenderName = strings.TrimSpace(config.SenderName)
	config.SenderPhone = strings.TrimSpace(config.SenderPhone)
	config.SenderCountry = strings.ToUpper(strings.TrimSpace(config.SenderCountry))
	config.SenderPostal = strings.TrimSpace(config.SenderPostal)
	config.SenderCity = strings.TrimSpace(config.SenderCity)
	config.SenderLine1 = strings.TrimSpace(config.SenderLine1)
	if config.BaseURL == "" || config.Username == "" || config.Password == "" || config.AccountNumber == "" || config.ProductCode == "" || config.SenderName == "" || config.SenderPhone == "" || config.SenderCountry == "" || config.SenderPostal == "" || config.SenderCity == "" || config.SenderLine1 == "" {
		return nil, fmt.Errorf("DHL Express credentials, account, product code and complete sender address are required")
	}
	parsed, err := url.Parse(config.BaseURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return nil, fmt.Errorf("DHL Express base URL must be an absolute HTTPS URL")
	}
	if config.PackageLength <= 0 || config.PackageWidth <= 0 || config.PackageHeight <= 0 {
		return nil, fmt.Errorf("DHL Express package dimensions must be positive")
	}
	if config.PriceScale < 0 || config.PriceScale > 6 {
		return nil, fmt.Errorf("DHL Express price scale must be between 0 and 6")
	}
	if config.HTTPClient == nil {
		config.HTTPClient = http.DefaultClient
	}
	return &Adapter{config: config}, nil
}

func (*Adapter) Code() string { return code }

// Quote calls MyDHL Rating and returns the configured product only. Selecting
// the product in configuration makes it possible to create the same service
// later from the durable delivery job, whose neutral payload deliberately does
// not expose provider product identifiers.
func (a *Adapter) Quote(ctx context.Context, request deliveryDomain.ShipmentQuoteRequest) ([]deliveryDomain.ShippingOption, error) {
	if err := a.validateDomesticDestination(request.Destination); err != nil {
		return nil, err
	}
	if _, err := money.NewMoney(0, request.Currency); err != nil {
		return nil, fmt.Errorf("DHL Express quote currency: %w", err)
	}
	packages, err := a.packages(request.Items)
	if err != nil {
		return nil, err
	}
	var response ratesResponse
	if err := a.doJSON(ctx, http.MethodPost, "/rates", a.rateRequest(request.Destination, packages), &response, ""); err != nil {
		return nil, err
	}
	for _, product := range response.Products {
		if product.ProductCode != a.config.ProductCode {
			continue
		}
		amount, err := product.price(request.Currency, a.config.PriceScale)
		if err != nil {
			return nil, err
		}
		option := deliveryDomain.ShippingOption{Code: a.config.ProductCode, Title: product.ProductName, Amount: amount}
		if option.Title == "" {
			option.Title = "DHL Express " + a.config.ProductCode
		}
		if product.DeliveryCapabilities.EstimatedDeliveryDateAndTime != "" {
			if estimated, err := time.Parse(time.RFC3339, product.DeliveryCapabilities.EstimatedDeliveryDateAndTime); err == nil {
				option.EstimatedAt = &estimated
			}
		}
		return []deliveryDomain.ShippingOption{option}, nil
	}
	return nil, fmt.Errorf("DHL Express product %q is unavailable for this shipment", a.config.ProductCode)
}

func (a *Adapter) CreateShipment(ctx context.Context, request deliveryDomain.CreateShipmentRequest) (deliveryDomain.ShipmentResult, error) {
	if request.OrderID == uuid.Nil || strings.TrimSpace(request.IdempotencyKey) == "" {
		return deliveryDomain.ShipmentResult{}, fmt.Errorf("DHL Express shipment requires order id and idempotency key")
	}
	if err := a.validateDomesticDestination(request.Destination); err != nil {
		return deliveryDomain.ShipmentResult{}, err
	}
	if err := request.DeclaredValue.Validate(); err != nil {
		return deliveryDomain.ShipmentResult{}, fmt.Errorf("DHL Express declared value: %w", err)
	}
	packages, err := a.packages(request.Items)
	if err != nil {
		return deliveryDomain.ShipmentResult{}, err
	}
	var response shipmentResponse
	if err := a.doJSON(ctx, http.MethodPost, "/shipments", a.shipmentRequest(request, packages), &response, request.IdempotencyKey); err != nil {
		return deliveryDomain.ShipmentResult{}, err
	}
	tracking := strings.TrimSpace(response.ShipmentTrackingNumber)
	if tracking == "" && len(response.Packages) > 0 {
		tracking = strings.TrimSpace(response.Packages[0].TrackingNumber)
	}
	if tracking == "" {
		return deliveryDomain.ShipmentResult{}, fmt.Errorf("DHL Express shipment response has no tracking number")
	}
	reference := strings.TrimSpace(response.ShipmentIdentificationNumber)
	if reference == "" {
		reference = tracking
	}
	return deliveryDomain.ShipmentResult{ProviderReference: reference, TrackingNumber: tracking}, nil
}

func (a *Adapter) Track(ctx context.Context, request deliveryDomain.TrackingRequest) (deliveryDomain.TrackingResult, error) {
	tracking := strings.TrimSpace(request.TrackingNumber)
	if tracking == "" {
		return deliveryDomain.TrackingResult{}, fmt.Errorf("DHL Express tracking number is required")
	}
	var response trackingResponse
	if err := a.doJSON(ctx, http.MethodGet, "/shipments/"+url.PathEscape(tracking)+"/tracking", nil, &response, ""); err != nil {
		return deliveryDomain.TrackingResult{}, err
	}
	if len(response.Shipments) == 0 {
		return deliveryDomain.TrackingResult{}, fmt.Errorf("DHL Express tracking response has no shipment")
	}
	shipment := response.Shipments[0]
	status := mapStatus(shipment.Status.StatusCode)
	occurredAt := parseDHLTime(shipment.Status.Timestamp)
	return deliveryDomain.TrackingResult{Status: status, OccurredAt: occurredAt}, nil
}

func (a *Adapter) validateDomesticDestination(destination deliveryDomain.Address) error {
	if strings.TrimSpace(destination.RecipientName) == "" || strings.TrimSpace(destination.RecipientPhone) == "" || strings.TrimSpace(destination.PostalCode) == "" || strings.TrimSpace(destination.City) == "" || strings.TrimSpace(destination.Line1) == "" || strings.TrimSpace(destination.CountryCode) == "" {
		return fmt.Errorf("DHL Express shipment requires recipient name, phone and complete postal address")
	}
	if strings.ToUpper(strings.TrimSpace(destination.CountryCode)) != a.config.SenderCountry {
		return fmt.Errorf("DHL Express international shipments require customs item data and are not enabled")
	}
	return nil
}

func (a *Adapter) rateRequest(destination deliveryDomain.Address, packages []map[string]any) any {
	return map[string]any{
		"customerDetails":            a.customerDetails(destination),
		"accounts":                   []map[string]string{{"typeCode": "shipper", "number": a.config.AccountNumber}},
		"plannedShippingDateAndTime": time.Now().UTC().Format(time.RFC3339),
		"unitOfMeasurement":          "metric",
		"isCustomsDeclarable":        false,
		"packages":                   packages,
	}
}

func (a *Adapter) shipmentRequest(request deliveryDomain.CreateShipmentRequest, packages []map[string]any) any {
	return map[string]any{
		"plannedShippingDateAndTime": time.Now().UTC().Format(time.RFC3339),
		"pickup":                     map[string]bool{"isRequested": false},
		"productCode":                a.config.ProductCode,
		"accounts":                   []map[string]string{{"typeCode": "shipper", "number": a.config.AccountNumber}},
		"customerDetails":            a.customerDetails(request.Destination),
		"content": map[string]any{
			"packages":            packages,
			"isCustomsDeclarable": false,
			"unitOfMeasurement":   "metric",
			"description":         "Order " + request.OrderID.String(),
		},
	}
}

func (a *Adapter) customerDetails(destination deliveryDomain.Address) any {
	return map[string]any{
		"shipperDetails": map[string]any{
			"postalAddress":      map[string]string{"postalCode": a.config.SenderPostal, "cityName": a.config.SenderCity, "countryCode": a.config.SenderCountry, "addressLine1": a.config.SenderLine1},
			"contactInformation": map[string]string{"fullName": a.config.SenderName, "phone": a.config.SenderPhone},
		},
		"receiverDetails": map[string]any{
			"postalAddress":      map[string]string{"postalCode": destination.PostalCode, "cityName": destination.City, "countryCode": strings.ToUpper(destination.CountryCode), "addressLine1": destination.Line1, "addressLine2": destination.Line2},
			"contactInformation": map[string]string{"fullName": destination.RecipientName, "phone": destination.RecipientPhone},
		},
	}
}

func (a *Adapter) packages(items []deliveryDomain.ShipmentItem) ([]map[string]any, error) {
	weightGrams := 0
	for _, item := range items {
		if item.Quantity > 0 && item.WeightGrams > 0 {
			weightGrams += item.Quantity * item.WeightGrams
		}
	}
	if weightGrams == 0 {
		return nil, fmt.Errorf("DHL Express shipment requires a positive item weight")
	}
	return []map[string]any{{"weight": float64(weightGrams) / 1000, "dimensions": map[string]float64{"length": a.config.PackageLength, "width": a.config.PackageWidth, "height": a.config.PackageHeight}}}, nil
}

func (a *Adapter) doJSON(ctx context.Context, method, path string, payload any, target any, idempotencyKey string) error {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("encode DHL Express request: %w", err)
		}
		body = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, a.config.BaseURL+path, body)
	if err != nil {
		return fmt.Errorf("create DHL Express request: %w", err)
	}
	req.SetBasicAuth(a.config.Username, a.config.Password)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if idempotencyKey != "" {
		req.Header.Set("Message-Reference", idempotencyKey)
		req.Header.Set("Message-Reference-Date", time.Now().UTC().Format(time.RFC3339))
	}
	response, err := a.config.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("call DHL Express: %w", err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read DHL Express response: %w", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return dhlError(response.StatusCode, raw)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("decode DHL Express response: %w", err)
	}
	return nil
}

type ratesResponse struct {
	Products []rateProduct `json:"products"`
}

type rateProduct struct {
	ProductCode string `json:"productCode"`
	ProductName string `json:"productName"`
	TotalPrice  []struct {
		CurrencyType string      `json:"currencyType"`
		Price        json.Number `json:"price"`
	} `json:"totalPrice"`
	DeliveryCapabilities struct {
		EstimatedDeliveryDateAndTime string `json:"estimatedDeliveryDateAndTime"`
	} `json:"deliveryCapabilities"`
}

func (p rateProduct) price(currency string, scale int) (money.Money, error) {
	for _, price := range p.TotalPrice {
		if strings.EqualFold(price.CurrencyType, currency) {
			amount, err := decimalMinorUnits(price.Price.String(), scale)
			if err != nil {
				return money.Money{}, fmt.Errorf("DHL Express price: %w", err)
			}
			return money.NewMoney(amount, strings.ToUpper(currency))
		}
	}
	return money.Money{}, fmt.Errorf("DHL Express product has no %s price", currency)
}

func decimalMinorUnits(value string, scale int) (int64, error) {
	ratio, ok := new(big.Rat).SetString(value)
	if !ok || ratio.Sign() < 0 {
		return 0, fmt.Errorf("invalid decimal price %q", value)
	}
	multiplier := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(scale)), nil)
	ratio.Mul(ratio, new(big.Rat).SetInt(multiplier))
	if ratio.Denom().Cmp(big.NewInt(1)) != 0 || !ratio.Num().IsInt64() {
		return 0, fmt.Errorf("price %q exceeds configured scale %d", value, scale)
	}
	return ratio.Num().Int64(), nil
}

type shipmentResponse struct {
	ShipmentTrackingNumber       string `json:"shipmentTrackingNumber"`
	ShipmentIdentificationNumber string `json:"shipmentIdentificationNumber"`
	Packages                     []struct {
		TrackingNumber string `json:"trackingNumber"`
	} `json:"packages"`
}

type trackingResponse struct {
	Shipments []struct {
		Status struct {
			StatusCode string `json:"statusCode"`
			Timestamp  string `json:"timestamp"`
		} `json:"status"`
	} `json:"shipments"`
}

func dhlError(status int, raw []byte) error {
	var response struct {
		Detail string `json:"detail"`
		Title  string `json:"title"`
	}
	if json.Unmarshal(raw, &response) == nil && (response.Detail != "" || response.Title != "") {
		return fmt.Errorf("DHL Express status %d: %s %s", status, response.Title, response.Detail)
	}
	return fmt.Errorf("DHL Express status %d", status)
}

func mapStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "delivered", "ok", "dl", "del":
		return "delivered"
	case "failure", "exception", "undelivered", "failed":
		return "failed"
	default:
		return "in_transit"
	}
}

func parseDHLTime(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

var _ deliveryDomain.Carrier = (*Adapter)(nil)
