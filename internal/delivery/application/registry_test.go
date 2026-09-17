package application

import (
	"context"
	"testing"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/delivery/domain"
)

func TestRegistrySelectsOnlyEnabledCarrier(t *testing.T) {
	correos := fakeCarrier{code: "correos"}
	novaPoshta := fakeCarrier{code: "novaposhta"}

	registry, err := NewRegistry([]string{"correos"}, "correos", correos, novaPoshta)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	if got := registry.Default(); got == nil || got.Code() != "correos" {
		t.Fatalf("Default() = %v, want correos", got)
	}
	if _, ok := registry.Get("novaposhta"); ok {
		t.Fatal("disabled carrier was registered")
	}
}

func TestRegistryRejectsConfiguredCarrierWithoutAdapter(t *testing.T) {
	if _, err := NewRegistry([]string{"correos"}, "correos"); err == nil {
		t.Fatal("NewRegistry() error = nil, want missing adapter error")
	}
}

type fakeCarrier struct{ code string }

func (c fakeCarrier) Code() string { return c.code }
func (fakeCarrier) Quote(context.Context, domain.ShipmentQuoteRequest) ([]domain.ShippingOption, error) {
	return nil, nil
}
func (fakeCarrier) CreateShipment(context.Context, domain.CreateShipmentRequest) (domain.ShipmentResult, error) {
	return domain.ShipmentResult{}, nil
}
func (fakeCarrier) Track(context.Context, domain.TrackingRequest) (domain.TrackingResult, error) {
	return domain.TrackingResult{}, nil
}
