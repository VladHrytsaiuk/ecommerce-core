// Package novaposhta translates Nova Poshta HTTP API calls to the neutral
// delivery Carrier port. It contains no checkout or order state transitions.
package novaposhta

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
	deliveryDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/delivery/domain"
)

const code = "novaposhta"

type Config struct {
	APIKey           string
	BaseURL          string
	SenderRef        string
	SenderCityRef    string
	SenderAddressRef string
	ContactSenderRef string
	SenderPhone      string
	HTTPClient       *http.Client
}
type Adapter struct{ config Config }

func New(config Config) (*Adapter, error) {
	if strings.TrimSpace(config.APIKey) == "" || strings.TrimSpace(config.BaseURL) == "" || strings.TrimSpace(config.SenderRef) == "" || strings.TrimSpace(config.SenderCityRef) == "" || strings.TrimSpace(config.SenderAddressRef) == "" || strings.TrimSpace(config.ContactSenderRef) == "" {
		return nil, fmt.Errorf("novaposhta API key and sender references are required")
	}
	if config.HTTPClient == nil {
		config.HTTPClient = http.DefaultClient
	}
	return &Adapter{config: config}, nil
}
func (*Adapter) Code() string { return code }

func (a *Adapter) Quote(ctx context.Context, request deliveryDomain.ShipmentQuoteRequest) ([]deliveryDomain.ShippingOption, error) {
	if request.Destination.LocalityID == "" || request.Currency != "UAH" {
		return nil, fmt.Errorf("novaposhta quote requires UAH and destination locality id")
	}
	weight := totalWeight(request.Items)
	data, err := a.call(ctx, "InternetDocument", "getDocumentPrice", map[string]any{"CitySender": a.config.SenderCityRef, "CityRecipient": request.Destination.LocalityID, "Weight": weight, "ServiceType": "WarehouseWarehouse", "CargoType": "Parcel", "SeatsAmount": "1", "Cost": "0"})
	if err != nil {
		return nil, err
	}
	var records []struct {
		Cost string `json:"Cost"`
	}
	if err := json.Unmarshal(data, &records); err != nil || len(records) == 0 {
		return nil, fmt.Errorf("decode novaposhta quote")
	}
	amount, err := hryvnias(records[0].Cost)
	if err != nil {
		return nil, err
	}
	return []deliveryDomain.ShippingOption{{Code: "warehouse", Title: "Nova Poshta", Amount: amount}}, nil
}

func (a *Adapter) CreateShipment(ctx context.Context, request deliveryDomain.CreateShipmentRequest) (deliveryDomain.ShipmentResult, error) {
	d := request.Destination
	if request.OrderID == uuid.Nil || strings.TrimSpace(request.IdempotencyKey) == "" || d.RecipientName == "" || d.RecipientPhone == "" || d.LocalityID == "" || d.ServicePointID == "" || request.DeclaredValue.Currency != "UAH" {
		return deliveryDomain.ShipmentResult{}, fmt.Errorf("novaposhta shipment requires recipient, locality, service point and UAH declared value")
	}
	data, err := a.call(ctx, "InternetDocument", "save", map[string]any{
		"PayerType": "Recipient", "PaymentMethod": "Cash", "CargoType": "Parcel",
		"Weight": totalWeight(request.Items), "ServiceType": "WarehouseWarehouse", "SeatsAmount": "1",
		// The provider has no HTTP idempotency header. The stable key is sent as
		// its client barcode reference; the future delivery outbox owns retries
		// and persists the resulting TTN before a retry is attempted.
		"InfoRegClientBarcodes": request.IdempotencyKey,
		"Description":           "Order " + request.OrderID.String(),
		"Cost":                  formatHryvnias(request.DeclaredValue.Amount),
		"CitySender":            a.config.SenderCityRef,
		"Sender":                a.config.SenderRef,
		"SenderAddress":         a.config.SenderAddressRef,
		"ContactSender":         a.config.ContactSenderRef,
		"SendersPhone":          a.config.SenderPhone,
		"CityRecipient":         d.LocalityID,
		"RecipientAddress":      d.ServicePointID,
		"RecipientName":         d.RecipientName,
		"RecipientsPhone":       d.RecipientPhone,
		"NewAddress":            1,
	})
	if err != nil {
		return deliveryDomain.ShipmentResult{}, err
	}
	var records []struct {
		Ref          string `json:"Ref"`
		IntDocNumber string `json:"IntDocNumber"`
	}
	if err := json.Unmarshal(data, &records); err != nil || len(records) == 0 {
		return deliveryDomain.ShipmentResult{}, fmt.Errorf("decode novaposhta shipment")
	}
	return deliveryDomain.ShipmentResult{ProviderReference: records[0].Ref, TrackingNumber: records[0].IntDocNumber}, nil
}
func (a *Adapter) Track(ctx context.Context, request deliveryDomain.TrackingRequest) (deliveryDomain.TrackingResult, error) {
	if request.TrackingNumber == "" {
		return deliveryDomain.TrackingResult{}, fmt.Errorf("novaposhta tracking number is required")
	}
	data, err := a.call(ctx, "TrackingDocument", "getStatusDocuments", map[string]any{"Documents": []map[string]string{{"DocumentNumber": request.TrackingNumber, "Phone": request.RecipientPhone}}})
	if err != nil {
		return deliveryDomain.TrackingResult{}, err
	}
	var records []struct {
		StatusCode string `json:"StatusCode"`
	}
	if err := json.Unmarshal(data, &records); err != nil || len(records) == 0 {
		return deliveryDomain.TrackingResult{}, fmt.Errorf("decode novaposhta tracking")
	}
	status := "in_transit"
	if records[0].StatusCode == "9" {
		status = "delivered"
	}
	return deliveryDomain.TrackingResult{Status: status}, nil
}
func (a *Adapter) call(ctx context.Context, model, method string, properties any) (json.RawMessage, error) {
	body, err := json.Marshal(map[string]any{"apiKey": a.config.APIKey, "modelName": model, "calledMethod": method, "methodProperties": properties})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.config.BaseURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := a.config.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("novaposhta status %d", res.StatusCode)
	}
	var response struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
		Errors  []string        `json:"errors"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, err
	}
	if !response.Success {
		return nil, fmt.Errorf("novaposhta API: %s", strings.Join(response.Errors, ", "))
	}
	return response.Data, nil
}
func totalWeight(items []deliveryDomain.ShipmentItem) string {
	total := 0
	for _, i := range items {
		total += i.WeightGrams * i.Quantity
	}
	if total <= 0 {
		total = 500
	}
	return fmt.Sprintf("%.3f", float64(total)/1000)
}
func hryvnias(value string) (money.Money, error) {
	parts := strings.Split(strings.TrimSpace(value), ".")
	if len(parts) > 2 {
		return money.Money{}, fmt.Errorf("invalid novaposhta price")
	}
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
	}
	for len(fraction) < 2 {
		fraction += "0"
	}
	if len(fraction) > 2 {
		return money.Money{}, fmt.Errorf("invalid novaposhta price")
	}
	amount, err := strconv.ParseInt(parts[0]+fraction, 10, 64)
	if err != nil {
		return money.Money{}, err
	}
	return money.New(amount, "UAH")
}
func formatHryvnias(amount int64) string { return fmt.Sprintf("%d.%02d", amount/100, amount%100) }

var _ deliveryDomain.Carrier = (*Adapter)(nil)
