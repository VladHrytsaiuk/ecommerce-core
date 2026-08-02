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

type fakeRepository struct{ reserved bool }

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
