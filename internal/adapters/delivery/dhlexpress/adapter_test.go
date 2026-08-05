package dhlexpress

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

func TestQuoteMapsNeutralRequestToMyDHL(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rates" || r.Method != http.MethodPost {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		username, password, ok := r.BasicAuth()
		if !ok || username != "user" || password != "password" {
			t.Fatalf("missing or invalid basic auth")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["isCustomsDeclarable"] != false || body["unitOfMeasurement"] != "metric" {
			t.Fatalf("unexpected rating body: %#v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"products":[{"productCode":"P","productName":"DHL Express Worldwide","totalPrice":[{"currencyType":"EUR","price":12.34}],"deliveryCapabilities":{"estimatedDeliveryDateAndTime":"2026-08-06T12:00:00Z"}}]}`))
	}))
	defer server.Close()

	adapter := testAdapter(t, server.URL, server.Client())
	options, err := adapter.Quote(context.Background(), deliveryDomain.ShipmentQuoteRequest{Currency: "EUR", Destination: testDestination(), Items: []deliveryDomain.ShipmentItem{{Quantity: 2, WeightGrams: 250}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(options) != 1 || options[0].Code != "P" || options[0].Amount.Amount != 1234 || options[0].Amount.Currency != "EUR" {
		t.Fatalf("options = %#v", options)
	}
}

func TestCreateShipmentUsesStableMessageReference(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/shipments" || r.Method != http.MethodPost {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Message-Reference"); got != "delivery-job-key" {
			t.Fatalf("Message-Reference = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"shipmentIdentificationNumber":"reference-1","shipmentTrackingNumber":"tracking-1"}`))
	}))
	defer server.Close()

	adapter := testAdapter(t, server.URL, server.Client())
	declared, err := money.New(1999, "EUR")
	if err != nil {
		t.Fatal(err)
	}
	shipment, err := adapter.CreateShipment(context.Background(), deliveryDomain.CreateShipmentRequest{OrderID: uuid.New(), IdempotencyKey: "delivery-job-key", Destination: testDestination(), Items: []deliveryDomain.ShipmentItem{{Quantity: 1, WeightGrams: 300}}, DeclaredValue: declared})
	if err != nil {
		t.Fatal(err)
	}
	if shipment.ProviderReference != "reference-1" || shipment.TrackingNumber != "tracking-1" {
		t.Fatalf("shipment = %#v", shipment)
	}
}

func TestRejectsInternationalShipmentWithoutCustomsData(t *testing.T) {
	adapter := testAdapter(t, "https://api.example.test", nil)
	destination := testDestination()
	destination.CountryCode = "UA"
	if _, err := adapter.Quote(context.Background(), deliveryDomain.ShipmentQuoteRequest{Currency: "EUR", Destination: destination}); err == nil {
		t.Fatal("Quote() error = nil, want customs-data guard")
	}
}

func testAdapter(t *testing.T, baseURL string, client *http.Client) *Adapter {
	t.Helper()
	adapter, err := New(Config{BaseURL: baseURL, Username: "user", Password: "password", AccountNumber: "123456789", ProductCode: "P", SenderName: "Store", SenderPhone: "+34910000000", SenderCountry: "ES", SenderPostal: "28001", SenderCity: "Madrid", SenderLine1: "Calle Example 1", PackageLength: 20, PackageWidth: 15, PackageHeight: 10, PriceScale: 2, HTTPClient: client})
	if err != nil {
		t.Fatal(err)
	}
	return adapter
}

func testDestination() deliveryDomain.Address {
	return deliveryDomain.Address{RecipientName: "Iryna Customer", RecipientPhone: "+34910000001", CountryCode: "ES", PostalCode: "08001", City: "Barcelona", Line1: "Carrer Example 2"}
}
