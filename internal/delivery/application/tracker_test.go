package application

import (
	"context"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/delivery/domain"
	"github.com/google/uuid"
	"testing"
)

func TestTrackerMapsCarrierStatusToActiveDelivery(t *testing.T) {
	store := &fakeTrackingStore{active: []domain.TrackingDelivery{{ID: uuid.New(), Provider: "fake", TrackingNumber: "TTN", RecipientPhone: "+1"}}}
	tracker := NewTracker(store, mustRegistry(t, trackingCarrier{}))
	if err := tracker.ReconcileOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.updated.ID != store.active[0].ID || store.updated.Status != "delivered" {
		t.Fatalf("updated=%+v", store.updated)
	}
}

type fakeTrackingStore struct {
	active  []domain.TrackingDelivery
	updated struct {
		ID uuid.UUID
		domain.TrackingResult
	}
}

func (f *fakeTrackingStore) ListActive(context.Context, int) ([]domain.TrackingDelivery, error) {
	return f.active, nil
}
func (f *fakeTrackingStore) UpdateStatus(_ context.Context, id uuid.UUID, r domain.TrackingResult) error {
	f.updated.ID = id
	f.updated.TrackingResult = r
	return nil
}

type trackingCarrier struct{}

func (trackingCarrier) Code() string { return "fake" }
func (trackingCarrier) Quote(context.Context, domain.ShipmentQuoteRequest) ([]domain.ShippingOption, error) {
	return nil, nil
}
func (trackingCarrier) CreateShipment(context.Context, domain.CreateShipmentRequest) (domain.ShipmentResult, error) {
	return domain.ShipmentResult{}, nil
}
func (trackingCarrier) Track(context.Context, domain.TrackingRequest) (domain.TrackingResult, error) {
	return domain.TrackingResult{Status: "delivered"}, nil
}
