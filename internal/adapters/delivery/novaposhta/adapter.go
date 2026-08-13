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
	"time"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
	deliveryDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/delivery/domain"
)

const code = "novaposhta"

const (
	warehouseTypeBranch   = "841339c7-591a-42e2-8233-7a0a00f0ed6f"
	warehouseTypePostomat = "f9316480-5f2d-425d-bc2c-ac7cd29decf0"
	warehouseTypeCargo    = "9a68df70-0267-42e2-8233-7a0a00f0ed6f"
)

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
		config.HTTPClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &Adapter{config: config}, nil
}
func (*Adapter) Code() string { return code }

func (a *Adapter) ListAreas(ctx context.Context) ([]deliveryDomain.Area, error) {
	data, err := a.call(ctx, "Address", "getAreas", map[string]any{})
	if err != nil {
		return nil, err
	}
	var records []struct {
		Ref, Description string
	}
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, fmt.Errorf("decode novaposhta areas: %w", err)
	}
	areas := make([]deliveryDomain.Area, 0, len(records))
	for _, record := range records {
		if record.Ref != "" && record.Description != "" {
			areas = append(areas, deliveryDomain.Area{ID: record.Ref, Name: record.Description})
		}
	}
	return areas, nil
}

func (a *Adapter) ListCities(ctx context.Context, areaID string) ([]deliveryDomain.City, error) {
	data, err := a.call(ctx, "Address", "getCities", map[string]any{"AreaRef": areaID})
	if err != nil {
		return nil, err
	}
	var records []struct {
		Ref         string `json:"Ref"`
		Description string `json:"Description"`
		Area        string `json:"Area"`
	}
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, fmt.Errorf("decode novaposhta cities: %w", err)
	}
	cities := make([]deliveryDomain.City, 0, len(records))
	for _, record := range records {
		if record.Ref != "" && record.Description != "" {
			cities = append(cities, deliveryDomain.City{ID: record.Ref, AreaID: record.Area, Name: record.Description})
		}
	}
	return cities, nil
}

func (a *Adapter) ListServicePoints(ctx context.Context, query deliveryDomain.ServicePointQuery) (deliveryDomain.ServicePointPage, error) {
	properties := map[string]any{"CityRef": query.CityID, "Page": strconv.Itoa(query.Page), "Limit": strconv.Itoa(query.Limit)}
	switch strings.ToLower(strings.TrimSpace(query.Kind)) {
	case "postomat":
		properties["TypeOfWarehouseRef"] = warehouseTypePostomat
	case "cargo":
		properties["TypeOfWarehouseRef"] = warehouseTypeCargo
	}
	response, err := a.callResponse(ctx, "Address", "getWarehouses", properties)
	if err != nil {
		return deliveryDomain.ServicePointPage{}, err
	}
	var records []struct {
		Ref             string `json:"Ref"`
		Description     string `json:"Description"`
		ShortAddress    string `json:"ShortAddress"`
		Number          string `json:"Number"`
		TypeOfWarehouse string `json:"TypeOfWarehouse"`
	}
	if err := json.Unmarshal(response.Data, &records); err != nil {
		return deliveryDomain.ServicePointPage{}, fmt.Errorf("decode novaposhta service points: %w", err)
	}
	points := make([]deliveryDomain.ServicePoint, 0, len(records))
	for _, record := range records {
		kind := servicePointKind(record.TypeOfWarehouse)
		if query.Kind == "branch" && kind != "branch" && kind != "cargo" {
			continue
		}
		if record.Ref != "" && record.Description != "" {
			points = append(points, deliveryDomain.ServicePoint{ID: record.Ref, Name: record.Description, Address: record.ShortAddress, Number: record.Number, Kind: kind})
		}
	}
	return deliveryDomain.ServicePointPage{Items: points, Page: query.Page, Limit: query.Limit, Total: totalCount(response.Info)}, nil
}

func servicePointKind(value string) string {
	switch value {
	case warehouseTypePostomat:
		return "postomat"
	case warehouseTypeCargo:
		return "cargo"
	case warehouseTypeBranch:
		return "branch"
	default:
		return "other"
	}
}

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

type responseEnvelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Errors  []string        `json:"errors"`
	Info    json.RawMessage `json:"info"`
}

func (a *Adapter) call(ctx context.Context, model, method string, properties any) (json.RawMessage, error) {
	response, err := a.callResponse(ctx, model, method, properties)
	if err != nil {
		return nil, err
	}
	return response.Data, nil
}

func (a *Adapter) callResponse(ctx context.Context, model, method string, properties any) (responseEnvelope, error) {
	body, err := json.Marshal(map[string]any{"apiKey": a.config.APIKey, "modelName": model, "calledMethod": method, "methodProperties": properties})
	if err != nil {
		return responseEnvelope{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.config.BaseURL, bytes.NewReader(body))
	if err != nil {
		return responseEnvelope{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := a.config.HTTPClient.Do(req)
	if err != nil {
		return responseEnvelope{}, fmt.Errorf("%w: novaposhta request: %v", deliveryDomain.ErrProviderUnavailable, err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return responseEnvelope{}, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return responseEnvelope{}, fmt.Errorf("%w: novaposhta status %d", deliveryDomain.ErrProviderUnavailable, res.StatusCode)
	}
	var response responseEnvelope
	if err := json.Unmarshal(raw, &response); err != nil {
		return responseEnvelope{}, err
	}
	if !response.Success {
		return responseEnvelope{}, fmt.Errorf("novaposhta API: %s", strings.Join(response.Errors, ", "))
	}
	return response, nil
}

func totalCount(raw json.RawMessage) int64 {
	var info struct {
		TotalCount json.Number `json:"totalCount"`
	}
	if len(raw) == 0 || json.Unmarshal(raw, &info) != nil {
		return 0
	}
	count, err := strconv.ParseInt(info.TotalCount.String(), 10, 64)
	if err != nil || count < 0 {
		return 0
	}
	return count
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
var _ deliveryDomain.LocationProvider = (*Adapter)(nil)
