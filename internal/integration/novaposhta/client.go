// Package novaposhta реалізує HTTP-клієнт для API Нової Пошти (v2.0).
package novaposhta

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shipment/domain"
)

// Константи для типів відділень Нової Пошти (TypeOfWarehouseRef)
const (
	// WarehouseTypeBranch — стандартне відділення НП
	WarehouseTypeBranch = "841339c7-591a-42e2-8233-7a0a00f0ed6f"
	// WarehouseTypePostomat — поштомат НП
	WarehouseTypePostomat = "f9316480-5f2d-425d-bc2c-ac7cd29decf0"
	// WarehouseTypeCargo — вантажне відділення НП
	WarehouseTypeCargo = "9a68df70-0267-42a8-bb5c-37f427e36ee4"
	// WarehouseTypeParcelShop — поштове відділення з обмеженням (Parcel Shop)
	WarehouseTypeParcelShop = "6f8c7162-4b72-4b0a-88e5-906948c6a92f"
)

// npRequest — структура запиту до API Нової Пошти
type npRequest struct {
	APIKey           string      `json:"apiKey"`
	ModelName        string      `json:"modelName"`
	CalledMethod     string      `json:"calledMethod"`
	MethodProperties interface{} `json:"methodProperties"`
}

// npResponse — структура відповіді від API Нової Пошти
type npResponse struct {
	Success  bool            `json:"success"`
	Data     json.RawMessage `json:"data"`
	Errors   []string        `json:"errors"`
	Warnings []string        `json:"warnings"`
}

// npArea — структура області з відповіді НП
type npArea struct {
	Ref         string `json:"Ref"`
	Description string `json:"Description"`
}

// npCity — структура міста з відповіді НП
type npCity struct {
	Ref         string `json:"Ref"`
	Description string `json:"Description"`
	AreaRef     string `json:"Area"`
}

// npWarehouse — структура відділення з відповіді НП
type npWarehouse struct {
	Ref             string `json:"Ref"`
	Description     string `json:"Description"`
	ShortAddress    string `json:"ShortAddress"`
	Number          string `json:"Number"`
	TypeOfWarehouse string `json:"TypeOfWarehouse"`
}

// Client — HTTP-клієнт для роботи з API Нової Пошти
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
	l          logger.Logger
}

// NewClient створює новий інстанс клієнта Нової Пошти
func NewClient(apiKey, baseURL string, l logger.Logger) *Client {
	return &Client{
		apiKey:  apiKey,
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
		l: l,
	}
}

// doRequest виконує уніфікований запит до API Нової Пошти
func (c *Client) doRequest(ctx context.Context, model, method string, props interface{}) (*npResponse, error) {
	reqBody := npRequest{
		APIKey:           c.apiKey,
		ModelName:        model,
		CalledMethod:     method,
		MethodProperties: props,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("novaposhta: failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("novaposhta: failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Трансформуємо мережеву помилку у доменну ErrProviderUnavailable
		return nil, fmt.Errorf("%w: connection failed: %v", domain.ErrProviderUnavailable, err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("novaposhta: failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		// Невірний HTTP код зазвичай означає, що сервіс лежить або заблокував нас
		return nil, fmt.Errorf("%w: unexpected status %d", domain.ErrProviderUnavailable, resp.StatusCode)
	}

	var npResp npResponse
	if err := json.Unmarshal(respBytes, &npResp); err != nil {
		return nil, fmt.Errorf("%w: failed to parse json response", domain.ErrExternalAPI)
	}

	if !npResp.Success {
		// Логічна помилка API (напр., невірний ключ або параметри)
		return nil, fmt.Errorf("%w: %v", domain.ErrExternalAPI, npResp.Errors)
	}

	return &npResp, nil
}

// GetAreas повертає список областей України
func (c *Client) GetAreas(ctx context.Context) ([]domain.Area, error) {
	resp, err := c.doRequest(ctx, "Address", "getAreas", struct{}{})
	if err != nil {
		return nil, err
	}

	var raw []npArea
	if err := json.Unmarshal(resp.Data, &raw); err != nil {
		return nil, fmt.Errorf("novaposhta: failed to parse areas: %w", err)
	}

	areas := make([]domain.Area, len(raw))
	for i, a := range raw {
		areas[i] = domain.Area{
			Ref:         a.Ref,
			Description: a.Description,
		}
	}

	c.l.Infow("Nova Poshta: fetched areas", "count", len(areas))
	return areas, nil
}

// GetCities повертає список міст для вказаної області
func (c *Client) GetCities(ctx context.Context, areaRef string) ([]domain.City, error) {
	props := map[string]string{
		"AreaRef": areaRef,
	}

	resp, err := c.doRequest(ctx, "Address", "getCities", props)
	if err != nil {
		return nil, err
	}

	var raw []npCity
	if err := json.Unmarshal(resp.Data, &raw); err != nil {
		return nil, fmt.Errorf("novaposhta: failed to parse cities: %w", err)
	}

	cities := make([]domain.City, len(raw))
	for i, ct := range raw {
		cities[i] = domain.City{
			Ref:         ct.Ref,
			Description: ct.Description,
			AreaRef:     ct.AreaRef,
		}
	}

	c.l.Infow("Nova Poshta: fetched cities", "area_ref", areaRef, "count", len(cities))
	return cities, nil
}

// GetWarehouses повертає список відділень/поштоматів для вказаного міста
func (c *Client) GetWarehouses(ctx context.Context, cityRef string, warehouseType string) ([]domain.Warehouse, error) {
	props := map[string]string{
		"CityRef": cityRef,
	}

	// Фільтруємо за типом, якщо вказано (реєстронезалежно)
	switch strings.ToLower(warehouseType) {
	case "branch":
		// Для branch ми не відправляємо фільтр в API, а фільтруємо результат вручну (див. нижче),
		// щоб об'єднати звичайні та вантажні відділення в одну категорію.
	case "postomat":
		props["TypeOfWarehouseRef"] = WarehouseTypePostomat
	case "cargo":
		props["TypeOfWarehouseRef"] = WarehouseTypeCargo
	}

	resp, err := c.doRequest(ctx, "Address", "getWarehouses", props)
	if err != nil {
		return nil, err
	}

	var raw []npWarehouse
	if err := json.Unmarshal(resp.Data, &raw); err != nil {
		return nil, fmt.Errorf("novaposhta: failed to parse warehouses: %w", err)
	}

	warehouses := make([]domain.Warehouse, 0, len(raw))
	for _, w := range raw {
		// Якщо було запитано "branch", ми фільтруємо вручну, щоб об'єднати типи
		if strings.ToLower(warehouseType) == "branch" {
			if w.TypeOfWarehouse != WarehouseTypeBranch &&
				w.TypeOfWarehouse != WarehouseTypeCargo &&
				w.TypeOfWarehouse != WarehouseTypeParcelShop {
				continue
			}
		}

		warehouses = append(warehouses, domain.Warehouse{
			Ref:             w.Ref,
			Description:     w.Description,
			ShortAddress:    w.ShortAddress,
			Number:          w.Number,
			TypeOfWarehouse: w.TypeOfWarehouse,
		})
	}

	c.l.Infow("Nova Poshta: fetched warehouses", "city_ref", cityRef, "type", warehouseType, "count", len(warehouses))
	return warehouses, nil
}

// ==========================================
// InternetDocument (TTN Creation)
// ==========================================

// CreateDocumentRequest параметри для створення ТТН
type CreateDocumentRequest struct {
	SenderRef        string
	SenderAddressRef string
	ContactSenderRef string
	SenderPhone      string
	RecipientName    string
	RecipientPhone   string
	CityRecipientRef string
	RecipientAddressRef string // warehouse ref
	Weight           string
	Description      string
	PayerType        string // "Recipient" / "Sender"
	PaymentMethod    string // "Cash" / "NonCash"
	ServiceType      string // "WarehouseWarehouse"
	SeatsAmount      string
	Cost             string // оголошена вартість у гривнях
}

// CreateDocumentResponse результат створення ТТН
type CreateDocumentResponse struct {
	Ref                   string          `json:"ref"`
	IntDocNumber          string          `json:"int_doc_number"`
	CostOnSite            string          `json:"cost_on_site"`
	EstimatedDeliveryDate string          `json:"estimated_delivery_date"`
	RawJSON               json.RawMessage `json:"raw_json"`
}

// npDocumentResult внутрішня структура відповіді НП для InternetDocument/save
type npDocumentResult struct {
	Ref                   string `json:"Ref"`
	IntDocNumber          string `json:"IntDocNumber"`
	CostOnSite            string `json:"CostOnSite"`
	EstimatedDeliveryDate string `json:"EstimatedDeliveryDate"`
}

// CreateInternetDocument створює ТТН через API Нової Пошти (InternetDocument/save)
func (c *Client) CreateInternetDocument(ctx context.Context, req CreateDocumentRequest) (*CreateDocumentResponse, error) {
	// Формуємо MethodProperties для InternetDocument/save
	props := map[string]interface{}{
		"PayerType":        req.PayerType,
		"PaymentMethod":    req.PaymentMethod,
		"DateTime":         time.Now().Format("02.01.2006"),
		"CargoType":        "Parcel",
		"Weight":           req.Weight,
		"ServiceType":      req.ServiceType,
		"SeatsAmount":      req.SeatsAmount,
		"Description":      req.Description,
		"Cost":             req.Cost,
		"CitySender":       "",
		"Sender":           req.SenderRef,
		"SenderAddress":    req.SenderAddressRef,
		"ContactSender":    req.ContactSenderRef,
		"SendersPhone":     req.SenderPhone,
		"CityRecipient":    req.CityRecipientRef,
		"Recipient":        "",
		"RecipientAddress": req.RecipientAddressRef,
		"ContactRecipient": "",
		"RecipientsPhone":  req.RecipientPhone,
	}

	// Для нереєстрованого отримувача НП дозволяє передавати дані напряму
	nameParts := strings.SplitN(req.RecipientName, " ", 3)
	recipientLastName := ""
	recipientFirstName := ""
	recipientMiddleName := ""
	if len(nameParts) >= 1 {
		recipientLastName = nameParts[0]
	}
	if len(nameParts) >= 2 {
		recipientFirstName = nameParts[1]
	}
	if len(nameParts) >= 3 {
		recipientMiddleName = nameParts[2]
	}

	// Використовуємо NewAddress для створення контрагента на льоту
	props["NewAddress"] = 1
	props["RecipientCityName"] = ""
	props["RecipientAddressName"] = ""
	props["RecipientName"] = req.RecipientName
	props["RecipientType"] = "PrivatePerson"
	props["CounterpartyRecipientDescription"] = recipientLastName
	props["RecipientsPhone"] = req.RecipientPhone

	// Override: використовуємо контрагент-рефи для отримувача через спрощений метод
	delete(props, "NewAddress")
	delete(props, "RecipientCityName")
	delete(props, "RecipientAddressName")
	delete(props, "CounterpartyRecipientDescription")
	delete(props, "RecipientName")

	// Створюємо контрагента-отримувача окремо
	recipientRef, contactRecipientRef, err := c.createRecipientCounterparty(ctx, recipientFirstName, recipientLastName, recipientMiddleName, req.RecipientPhone)
	if err != nil {
		return nil, fmt.Errorf("failed to create recipient counterparty: %w", err)
	}

	props["Recipient"] = recipientRef
	props["ContactRecipient"] = contactRecipientRef

	c.l.Infow("Nova Poshta: creating InternetDocument",
		"sender_ref", req.SenderRef,
		"recipient_phone", req.RecipientPhone,
		"city_recipient", req.CityRecipientRef,
		"warehouse_recipient", req.RecipientAddressRef,
	)

	resp, err := c.doRequest(ctx, "InternetDocument", "save", props)
	if err != nil {
		return nil, fmt.Errorf("failed to create internet document: %w", err)
	}

	var results []npDocumentResult
	if err := json.Unmarshal(resp.Data, &results); err != nil {
		return nil, fmt.Errorf("novaposhta: failed to parse document response: %w", err)
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("%w: empty response from InternetDocument/save", domain.ErrExternalAPI)
	}

	doc := results[0]

	c.l.Infow("Nova Poshta: InternetDocument created",
		"ref", doc.Ref,
		"int_doc_number", doc.IntDocNumber,
		"cost_on_site", doc.CostOnSite,
	)

	return &CreateDocumentResponse{
		Ref:                   doc.Ref,
		IntDocNumber:          doc.IntDocNumber,
		CostOnSite:            doc.CostOnSite,
		EstimatedDeliveryDate: doc.EstimatedDeliveryDate,
		RawJSON:               resp.Data,
	}, nil
}

// npCounterpartyResult внутрішня структура для Counterparty/save
type npCounterpartyResult struct {
	Ref            string `json:"Ref"`
	ContactPerson  struct {
		Data []struct {
			Ref string `json:"Ref"`
		} `json:"data"`
	} `json:"ContactPerson"`
}

// createRecipientCounterparty створює контрагента-отримувача через Counterparty/save
func (c *Client) createRecipientCounterparty(ctx context.Context, firstName, lastName, middleName, phone string) (recipientRef, contactRef string, err error) {
	props := map[string]string{
		"FirstName":       firstName,
		"LastName":        lastName,
		"MiddleName":      middleName,
		"Phone":           phone,
		"CounterpartyType": "PrivatePerson",
		"CounterpartyProperty": "Recipient",
	}

	resp, err := c.doRequest(ctx, "Counterparty", "save", props)
	if err != nil {
		return "", "", err
	}

	// Парсимо відповідь: НП повертає масив з одним елементом
	var raw []json.RawMessage
	if err := json.Unmarshal(resp.Data, &raw); err != nil {
		return "", "", fmt.Errorf("novaposhta: failed to parse counterparty response: %w", err)
	}
	if len(raw) == 0 {
		return "", "", fmt.Errorf("%w: empty counterparty response", domain.ErrExternalAPI)
	}

	// Витягуємо Ref контрагента
	var counterparty struct {
		Ref           string `json:"Ref"`
		ContactPerson struct {
			Data []struct {
				Ref string `json:"Ref"`
			} `json:"data"`
		} `json:"ContactPerson"`
	}
	if err := json.Unmarshal(raw[0], &counterparty); err != nil {
		return "", "", fmt.Errorf("novaposhta: failed to parse counterparty data: %w", err)
	}

	recipientRef = counterparty.Ref
	if len(counterparty.ContactPerson.Data) > 0 {
		contactRef = counterparty.ContactPerson.Data[0].Ref
	} else {
		contactRef = recipientRef // fallback
	}

	c.l.Infow("Nova Poshta: recipient counterparty created",
		"recipient_ref", recipientRef,
		"contact_ref", contactRef,
	)

	return recipientRef, contactRef, nil
}

// DocumentTrackingResponse структура відповіді трекінгу НП
type DocumentTrackingResponse struct {
	StatusCode       string `json:"StatusCode"`       // "9", "1", "2"
	Status           string `json:"Status"`           // Текстовий статус
	DocumentNumber   string `json:"DocumentNumber"`
	RecipientAddress string `json:"RecipientAddress"`
	CityRecipient    string `json:"CityRecipient"`
	ActualDeliveryDate string `json:"ActualDeliveryDate"`
}

// GetDocumentTracking повертає статус відправлення за номером ТТН та телефоном
func (c *Client) GetDocumentTracking(ctx context.Context, ttnNumber, phone string) (*DocumentTrackingResponse, error) {
	props := map[string]interface{}{
		"Documents": []map[string]string{
			{
				"DocumentNumber": ttnNumber,
				"Phone":          phone,
			},
		},
	}

	resp, err := c.doRequest(ctx, "TrackingDocument", "getStatusDocuments", props)
	if err != nil {
		return nil, fmt.Errorf("novaposhta tracking failed: %w", err)
	}

	var results []DocumentTrackingResponse
	if err := json.Unmarshal(resp.Data, &results); err != nil {
		return nil, fmt.Errorf("novaposhta tracking json parse error: %w", err)
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("novaposhta tracking: no data returned for TTN %s", ttnNumber)
	}

	c.l.Infow("Nova Poshta tracking success", "ttn", ttnNumber, "status_code", results[0].StatusCode, "status_text", results[0].Status)
	return &results[0], nil
}
