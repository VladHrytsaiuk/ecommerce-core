package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/inventory/domain"
	"github.com/google/uuid"
)

func TestExternalModeBlocksStockAdjustmentButAllowsReservation(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(domain.ModeExternal, repo)
	if err := service.Adjust(context.Background(), uuid.New(), uuid.New(), 1); !errors.Is(err, domain.ErrStockReadOnly) {
		t.Fatalf("Adjust() = %v", err)
	}
	_, err := service.Reserve(context.Background(), domain.ReservationRequest{IdempotencyKey: uuid.New(), VariantID: uuid.New(), WarehouseID: uuid.New(), Quantity: 1, ExpiresAt: time.Now().Add(time.Minute)})
	if err != nil || !repo.reserved {
		t.Fatalf("Reserve() = %v, reserved=%v", err, repo.reserved)
	}
}

func TestExternalQuantityReplacementIsOnlyAvailableInExternalMode(t *testing.T) {
	variantID, warehouseID := uuid.New(), uuid.New()
	externalRepo := &fakeRepository{}
	if err := NewService(domain.ModeExternal, externalRepo).ReplaceExternalQuantity(context.Background(), variantID, warehouseID, 12); err != nil || externalRepo.replaced != 12 {
		t.Fatalf("ReplaceExternalQuantity() = %v, quantity=%d", err, externalRepo.replaced)
	}
	if err := NewService(domain.ModeInternal, &fakeRepository{}).ReplaceExternalQuantity(context.Background(), variantID, warehouseID, 12); !errors.Is(err, domain.ErrExternalImportDisabled) {
		t.Fatalf("internal ReplaceExternalQuantity() = %v", err)
	}
}

type fakeRepository struct {
	reserved bool
	replaced int
}

func (r *fakeRepository) Reserve(_ context.Context, q domain.ReservationRequest) (*domain.Reservation, error) {
	r.reserved = true
	return &domain.Reservation{ID: uuid.New(), IdempotencyKey: q.IdempotencyKey}, nil
}
func (r *fakeRepository) ReserveBatch(_ context.Context, qs []domain.ReservationRequest) ([]domain.Reservation, error) {
	r.reserved = true
	reservations := make([]domain.Reservation, 0, len(qs))
	for _, q := range qs {
		reservations = append(reservations, domain.Reservation{ID: uuid.New(), IdempotencyKey: q.IdempotencyKey})
	}
	return reservations, nil
}
func (r *fakeRepository) Release(context.Context, uuid.UUID) error                { return nil }
func (r *fakeRepository) Commit(context.Context, uuid.UUID, uuid.UUID) error      { return nil }
func (r *fakeRepository) Adjust(context.Context, uuid.UUID, uuid.UUID, int) error { return nil }
func (r *fakeRepository) ReplaceQuantity(_ context.Context, _, _ uuid.UUID, quantity int) error {
	r.replaced = quantity
	return nil
}
