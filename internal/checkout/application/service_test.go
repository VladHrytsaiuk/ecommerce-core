package application

import (
	"context"
	"testing"
	"time"

	checkoutDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/checkout/domain"
	inventoryDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/inventory/domain"
	"github.com/google/uuid"
)

func TestPreparePaymentAggregatesDuplicateLinesIntoOneAtomicBatch(t *testing.T) {
	inventory := &fakeInventory{}
	service := NewService(inventory)
	variant, warehouse := uuid.New(), uuid.New()
	prepared, err := service.PreparePayment(context.Background(), checkoutDomain.PrepareRequest{CheckoutID: uuid.New(), ExpiresAt: time.Now().Add(time.Minute), Lines: []checkoutDomain.Line{{VariantID: variant, WarehouseID: warehouse, Quantity: 1}, {VariantID: variant, WarehouseID: warehouse, Quantity: 2}}})
	if err != nil {
		t.Fatalf("PreparePayment() error = %v", err)
	}
	if len(inventory.batch) != 1 || inventory.batch[0].Quantity != 3 || len(prepared.ReservationIDs) != 1 {
		t.Fatalf("batch = %+v, prepared = %+v", inventory.batch, prepared)
	}
}

type fakeInventory struct {
	batch []inventoryDomain.ReservationRequest
}

func (f *fakeInventory) Reserve(context.Context, inventoryDomain.ReservationRequest) (*inventoryDomain.Reservation, error) {
	return nil, nil
}
func (f *fakeInventory) ReserveBatch(_ context.Context, qs []inventoryDomain.ReservationRequest) ([]inventoryDomain.Reservation, error) {
	f.batch = qs
	result := make([]inventoryDomain.Reservation, 0, len(qs))
	for _, q := range qs {
		result = append(result, inventoryDomain.Reservation{ID: uuid.New(), IdempotencyKey: q.IdempotencyKey})
	}
	return result, nil
}
func (f *fakeInventory) Release(context.Context, uuid.UUID) error                { return nil }
func (f *fakeInventory) Commit(context.Context, uuid.UUID, uuid.UUID) error      { return nil }
func (f *fakeInventory) Adjust(context.Context, uuid.UUID, uuid.UUID, int) error { return nil }
