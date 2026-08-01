package novaposhta

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"go.uber.org/zap"
)

type noopLogger struct{}

func (n *noopLogger) Debug(msg string, fields ...zap.Field)       {}
func (n *noopLogger) Info(msg string, fields ...zap.Field)        {}
func (n *noopLogger) Warn(msg string, fields ...zap.Field)        {}
func (n *noopLogger) Error(msg string, fields ...zap.Field)       {}
func (n *noopLogger) Fatal(msg string, fields ...zap.Field)       {}
func (n *noopLogger) Debugf(template string, args ...interface{}) {}
func (n *noopLogger) Infof(template string, args ...interface{})  {}
func (n *noopLogger) Warnf(template string, args ...interface{})  {}
func (n *noopLogger) Errorf(template string, args ...interface{}) {}
func (n *noopLogger) Fatalf(template string, args ...interface{}) {}
func (n *noopLogger) Debugw(msg string, kvs ...interface{})       {}
func (n *noopLogger) Infow(msg string, kvs ...interface{})        {}
func (n *noopLogger) Warnw(msg string, kvs ...interface{})        {}
func (n *noopLogger) Errorw(msg string, kvs ...interface{})       {}
func (n *noopLogger) Fatalw(msg string, kvs ...interface{})       {}
func (n *noopLogger) With(fields ...zap.Field) logger.Logger      { return n }
func (n *noopLogger) Sync() error                                 { return nil }

func TestClient_GetAreas(t *testing.T) {
	mockResponse := npResponse{
		Success: true,
		Data: []byte(`[
			{"Ref": "area-ref-1", "Description": "Київська"},
			{"Ref": "area-ref-2", "Description": "Львівська"}
		]`),
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var req npRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)

		assert.Equal(t, "test-api-key", req.APIKey)
		assert.Equal(t, "Address", req.ModelName)
		assert.Equal(t, "getAreas", req.CalledMethod)

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(mockResponse)
	}))
	defer server.Close()

	l := &noopLogger{}
	client := NewClient("test-api-key", server.URL, l)

	areas, err := client.GetAreas(context.Background())
	require.NoError(t, err)

	assert.Len(t, areas, 2)
	assert.Equal(t, "area-ref-1", areas[0].Ref)
	assert.Equal(t, "Київська", areas[0].Description)
	assert.Equal(t, "area-ref-2", areas[1].Ref)
	assert.Equal(t, "Львівська", areas[1].Description)
}

func TestClient_GetCities(t *testing.T) {
	mockResponse := npResponse{
		Success: true,
		Data: []byte(`[
			{"Ref": "city-ref-1", "Description": "Київ", "Area": "area-ref-1"},
			{"Ref": "city-ref-2", "Description": "Бровари", "Area": "area-ref-1"}
		]`),
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req npRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)

		assert.Equal(t, "Address", req.ModelName)
		assert.Equal(t, "getCities", req.CalledMethod)

		props, ok := req.MethodProperties.(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "area-ref-1", props["AreaRef"])

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(mockResponse)
	}))
	defer server.Close()

	l := &noopLogger{}
	client := NewClient("test-api-key", server.URL, l)

	cities, err := client.GetCities(context.Background(), "area-ref-1")
	require.NoError(t, err)

	assert.Len(t, cities, 2)
	assert.Equal(t, "city-ref-1", cities[0].Ref)
	assert.Equal(t, "Київ", cities[0].Description)
	assert.Equal(t, "area-ref-1", cities[0].AreaRef)
}

func TestClient_GetWarehouses(t *testing.T) {
	t.Run("success without specific type", func(t *testing.T) {
		mockResponse := npResponse{
			Success: true,
			Data: []byte(`[
				{
					"Ref": "wh-ref-1", 
					"Description": "Відділення №1", 
					"ShortAddress": "вул. Хрещатик, 1",
					"Number": "1",
					"TypeOfWarehouse": "Branch"
				}
			]`),
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req npRequest
			json.NewDecoder(r.Body).Decode(&req)

			assert.Equal(t, "Address", req.ModelName)
			assert.Equal(t, "getWarehouses", req.CalledMethod)

			props, ok := req.MethodProperties.(map[string]interface{})
			require.True(t, ok)
			assert.Equal(t, "city-ref-1", props["CityRef"])
			assert.Nil(t, props["TypeOfWarehouseRef"])

			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(mockResponse)
		}))
		defer server.Close()

		l := &noopLogger{}
		client := NewClient("test-api-key", server.URL, l)

		warehouses, err := client.GetWarehouses(context.Background(), "city-ref-1", "")
		require.NoError(t, err)

		assert.Len(t, warehouses, 1)
		assert.Equal(t, "wh-ref-1", warehouses[0].Ref)
		assert.Equal(t, "Відділення №1", warehouses[0].Description)
		assert.Equal(t, "вул. Хрещатик, 1", warehouses[0].ShortAddress)
		assert.Equal(t, "1", warehouses[0].Number)
		assert.Equal(t, "Branch", warehouses[0].TypeOfWarehouse)
	})

	t.Run("success with branch type (combined)", func(t *testing.T) {
		mockResponse := npResponse{
			Success: true,
			Data: []byte(`[
				{"Ref": "1", "Description": "Branch", "TypeOfWarehouse": "841339c7-591a-42e2-8233-7a0a00f0ed6f"},
				{"Ref": "2", "Description": "Cargo", "TypeOfWarehouse": "9a68df70-0267-42a8-bb5c-37f427e36ee4"},
				{"Ref": "3", "Description": "ParcelShop", "TypeOfWarehouse": "6f8c7162-4b72-4b0a-88e5-906948c6a92f"},
				{"Ref": "4", "Description": "Postomat", "TypeOfWarehouse": "f9316480-5f2d-425d-bc2c-ac7cd29decf0"}
			]`),
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req npRequest
			json.NewDecoder(r.Body).Decode(&req)

			// For "branch", we shouldn't see the filter in API
			assert.Nil(t, req.MethodProperties.(map[string]interface{})["TypeOfWarehouseRef"])

			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(mockResponse)
		}))
		defer server.Close()

		l := &noopLogger{}
		client := NewClient("test-api-key", server.URL, l)

		warehouses, err := client.GetWarehouses(context.Background(), "city-ref-1", "branch")
		require.NoError(t, err)

		// Should include 3 (Branch, Cargo, ParcelShop) but exclude Postomat
		assert.Len(t, warehouses, 3)
		assert.Equal(t, "1", warehouses[0].Ref)
		assert.Equal(t, "2", warehouses[1].Ref)
		assert.Equal(t, "3", warehouses[2].Ref)
	})

	t.Run("success with postomat type case-insensitive", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req npRequest
			json.NewDecoder(r.Body).Decode(&req)

			props, _ := req.MethodProperties.(map[string]interface{})
			assert.Equal(t, WarehouseTypePostomat, props["TypeOfWarehouseRef"])

			json.NewEncoder(w).Encode(npResponse{Success: true, Data: []byte(`[]`)})
		}))
		defer server.Close()

		l := &noopLogger{}
		client := NewClient("test-api-key", server.URL, l)

		// Test with mixed case "Postomat"
		_, err := client.GetWarehouses(context.Background(), "city-ref-1", "Postomat")
		require.NoError(t, err)
	})

	t.Run("API error", func(t *testing.T) {
		mockResponse := npResponse{
			Success: false,
			Errors:  []string{"API Error Details"},
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(mockResponse)
		}))
		defer server.Close()

		l := &noopLogger{}
		client := NewClient("test-api-key", server.URL, l)

		_, err := client.GetAreas(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "external shipping api returned error")
		assert.Contains(t, err.Error(), "API Error Details")
	})

	t.Run("HTTP error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		l := &noopLogger{}
		client := NewClient("test-api-key", server.URL, l)

		_, err := client.GetAreas(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "shipping provider service is temporarily unavailable")
		assert.Contains(t, err.Error(), "unexpected status 500")
	})
}

func TestClient_CreateInternetDocument(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// First request will be for creating counterparty (recipient)
		// Second request will be for creating internet document
		requestCount := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestCount++
			var req npRequest
			json.NewDecoder(r.Body).Decode(&req)

			w.WriteHeader(http.StatusOK)
			if requestCount == 1 {
				// Counterparty response
				assert.Equal(t, "Counterparty", req.ModelName)
				assert.Equal(t, "save", req.CalledMethod)
				mockResponse := npResponse{
					Success: true,
					Data: []byte(`[
						{
							"Ref": "recipient-ref-1",
							"ContactPerson": {
								"data": [{"Ref": "contact-ref-1"}]
							}
						}
					]`),
				}
				json.NewEncoder(w).Encode(mockResponse)
			} else if requestCount == 2 {
				// InternetDocument response
				assert.Equal(t, "InternetDocument", req.ModelName)
				assert.Equal(t, "save", req.CalledMethod)
				
				props, _ := req.MethodProperties.(map[string]interface{})
				assert.Equal(t, "recipient-ref-1", props["Recipient"])
				assert.Equal(t, "contact-ref-1", props["ContactRecipient"])

				mockResponse := npResponse{
					Success: true,
					Data: []byte(`[
						{
							"Ref": "doc-ref-1",
							"IntDocNumber": "20450000000001",
							"CostOnSite": "50",
							"EstimatedDeliveryDate": "12.08.2026"
						}
					]`),
				}
				json.NewEncoder(w).Encode(mockResponse)
			}
		}))
		defer server.Close()

		l := &noopLogger{}
		client := NewClient("test-api-key", server.URL, l)

		req := CreateDocumentRequest{
			SenderRef:           "sender-ref",
			SenderAddressRef:    "sender-addr",
			ContactSenderRef:    "sender-contact",
			SenderPhone:         "380501112233",
			RecipientName:       "Іванов Іван Іванович",
			RecipientPhone:      "380671112233",
			CityRecipientRef:    "city-ref",
			RecipientAddressRef: "warehouse-ref",
			Weight:              "1",
			Description:         "Одяг",
			PayerType:           "Recipient",
			PaymentMethod:       "Cash",
			ServiceType:         "WarehouseWarehouse",
			SeatsAmount:         "1",
			Cost:                "1000",
		}

		resp, err := client.CreateInternetDocument(context.Background(), req)
		require.NoError(t, err)
		assert.Equal(t, 2, requestCount)
		assert.NotNil(t, resp)
		assert.Equal(t, "doc-ref-1", resp.Ref)
		assert.Equal(t, "20450000000001", resp.IntDocNumber)
		assert.Equal(t, "50", resp.CostOnSite)
		assert.Equal(t, "12.08.2026", resp.EstimatedDeliveryDate)
	})

	t.Run("Counterparty_Fail", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			mockResponse := npResponse{
				Success: false,
				Errors: []string{"Invalid phone number"},
			}
			json.NewEncoder(w).Encode(mockResponse)
		}))
		defer server.Close()

		l := &noopLogger{}
		client := NewClient("test-api-key", server.URL, l)

		_, err := client.CreateInternetDocument(context.Background(), CreateDocumentRequest{
			RecipientName: "Іванов",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create recipient counterparty")
	})
}
