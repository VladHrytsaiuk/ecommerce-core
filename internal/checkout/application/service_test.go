package application

import (
	"context"
	"errors"
	"testing"
	"time"

	catalogDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/domain"
	checkoutDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/checkout/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/tax"
	inventoryDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/inventory/domain"
	"github.com/google/uuid"
)

func TestPreparePaymentAggregatesDuplicateLinesIntoOneAtomicBatch(t *testing.T) {
	inventory := &fakeInventory{}
	price, _ := money.New(1000, "EUR")
	service := NewService(inventory, &fakeVariantFinder{price: price}, mustTaxPolicy(t, tax.ModeVATExcluded, 20))
	variant, warehouse := uuid.New(), uuid.New()
	prepared, err := service.PreparePayment(context.Background(), checkoutDomain.PrepareRequest{CheckoutID: uuid.New(), Locale: "es", ExpiresAt: time.Now().Add(time.Minute), Lines: []checkoutDomain.Line{{VariantID: variant, WarehouseID: warehouse, Quantity: 1}, {VariantID: variant, WarehouseID: warehouse, Quantity: 2}}})
	if err != nil {
		t.Fatalf("PreparePayment() error = %v", err)
	}
	if len(inventory.batch) != 1 || inventory.batch[0].Quantity != 3 || len(prepared.ReservationIDs) != 1 || len(prepared.Items) != 1 || prepared.Subtotal.Amount != 3000 || prepared.Tax.Amount != 600 || prepared.Total.Amount != 3600 {
		t.Fatalf("batch = %+v, prepared = %+v", inventory.batch, prepared)
	}
}

func TestPreparePaymentDoesNotReserveWhenCatalogVariantIsUnavailable(t *testing.T) {
	inventory := &fakeInventory{}
	price, _ := money.New(1000, "EUR")
	service := NewService(inventory, &fakeVariantFinder{price: price, err: errors.New("variant unavailable")}, mustTaxPolicy(t, tax.ModeNone, 0))

	_, err := service.PreparePayment(context.Background(), checkoutDomain.PrepareRequest{CheckoutID: uuid.New(), Locale: "es", ExpiresAt: time.Now().Add(time.Minute), Lines: []checkoutDomain.Line{{VariantID: uuid.New(), WarehouseID: uuid.New(), Quantity: 1}}})
	if err == nil {
		t.Fatal("PreparePayment() error = nil, want catalog error")
	}
	if len(inventory.batch) != 0 {
		t.Fatalf("ReserveBatch() was called before catalog validation: %+v", inventory.batch)
	}
}

type fakeVariantFinder struct {
	price money.Money
	err   error
}

func (f *fakeVariantFinder) FindActiveForCheckout(_ context.Context, variantID uuid.UUID, _ string) (*catalogDomain.CheckoutVariant, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &catalogDomain.CheckoutVariant{VariantID: variantID, ProductID: uuid.New(), ProductName: "Cream", SKU: "CREAM-50", UnitPrice: f.price}, nil
}

func mustTaxPolicy(t *testing.T, mode tax.Mode, rate int) tax.Calculator {
	t.Helper()
	policy, err := tax.NewPolicy(mode, rate)
	if err != nil {
		t.Fatalf("NewPolicy() error = %v", err)
	}
	return policy
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
