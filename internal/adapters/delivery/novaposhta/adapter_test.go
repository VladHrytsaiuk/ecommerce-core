package novaposhta

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
	deliveryDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/delivery/domain"
)

func TestQuoteMapsNeutralRequestToNovaPoshtaAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request := decodeRequest(t, r)
		if request.ModelName != "InternetDocument" || request.Method != "getDocumentPrice" {
			t.Fatalf("request = %+v", request)
		}
		if request.Properties["CitySender"] != "sender-city" || request.Properties["CityRecipient"] != "recipient-city" || request.Properties["Weight"] != "0.750" {
			t.Fatalf("properties = %#v", request.Properties)
		}
		_, _ = w.Write([]byte(`{"success":true,"data":[{"Cost":"75.50"}]}`))
	}))
	defer server.Close()

	adapter := newAdapter(t, server)
	options, err := adapter.Quote(context.Background(), deliveryDomain.ShipmentQuoteRequest{
		Destination: deliveryDomain.Address{LocalityID: "recipient-city"},
		Currency:    "UAH",
		Items:       []deliveryDomain.ShipmentItem{{Quantity: 3, WeightGrams: 250}},
	})
	if err != nil {
		t.Fatalf("Quote() error = %v", err)
	}
	if len(options) != 1 || options[0].Code != "warehouse" || options[0].Amount.Amount != 7550 || options[0].Amount.Currency != "UAH" {
		t.Fatalf("options = %+v", options)
	}
}

func TestCreateShipmentMapsNeutralRequestAndStableKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request := decodeRequest(t, r)
		if request.ModelName != "InternetDocument" || request.Method != "save" {
			t.Fatalf("request = %+v", request)
		}
		if request.Properties["Sender"] != "sender" || request.Properties["CitySender"] != "sender-city" || request.Properties["RecipientAddress"] != "branch-7" || request.Properties["InfoRegClientBarcodes"] != "shipment-42" {
			t.Fatalf("properties = %#v", request.Properties)
		}
		_, _ = w.Write([]byte(`{"success":true,"data":[{"Ref":"document-ref","IntDocNumber":"20400000000000"}]}`))
	}))
	defer server.Close()

	adapter := newAdapter(t, server)
	declaredValue, _ := money.New(12345, "UAH")
	shipment, err := adapter.CreateShipment(context.Background(), deliveryDomain.CreateShipmentRequest{
		OrderID:        uuid.New(),
		IdempotencyKey: "shipment-42",
		DeclaredValue:  declaredValue,
		Destination: deliveryDomain.Address{
			RecipientName: "Iryna Customer", RecipientPhone: "+380501112233",
			LocalityID: "recipient-city", ServicePointID: "branch-7",
		},
		Items: []deliveryDomain.ShipmentItem{{Quantity: 1, WeightGrams: 500}},
	})
	if err != nil {
		t.Fatalf("CreateShipment() error = %v", err)
	}
	if shipment.ProviderReference != "document-ref" || shipment.TrackingNumber != "20400000000000" {
		t.Fatalf("shipment = %+v", shipment)
	}
}

func TestTrackMapsDeliveredProviderStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request := decodeRequest(t, r)
		if request.ModelName != "TrackingDocument" || request.Method != "getStatusDocuments" {
			t.Fatalf("request = %+v", request)
		}
		_, _ = w.Write([]byte(`{"success":true,"data":[{"StatusCode":"9"}]}`))
	}))
	defer server.Close()

	adapter := newAdapter(t, server)
	tracking, err := adapter.Track(context.Background(), deliveryDomain.TrackingRequest{TrackingNumber: "20400000000000", RecipientPhone: "+380501112233"})
	if err != nil {
		t.Fatalf("Track() error = %v", err)
	}
	if tracking.Status != "delivered" {
		t.Fatalf("tracking = %+v", tracking)
	}
}

func newAdapter(t *testing.T, server *httptest.Server) *Adapter {
	t.Helper()
	adapter, err := New(Config{
		APIKey: "test-key", BaseURL: server.URL, SenderRef: "sender", SenderCityRef: "sender-city",
		SenderAddressRef: "sender-branch", ContactSenderRef: "sender-contact", SenderPhone: "+380501234567",
		HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return adapter
}

type apiRequest struct {
	ModelName  string         `json:"modelName"`
	Method     string         `json:"calledMethod"`
	Properties map[string]any `json:"methodProperties"`
}

func decodeRequest(t *testing.T, r *http.Request) apiRequest {
	t.Helper()
	defer r.Body.Close()
	var request apiRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		t.Fatal(err)
	}
	return request
}
