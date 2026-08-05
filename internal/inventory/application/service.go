package application

import (
	"context"
	"fmt"
	"time"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/inventory/domain"
	"github.com/google/uuid"
)

type Service struct {
	mode domain.Mode
	repo domain.Repository
}

func NewService(mode domain.Mode, repo domain.Repository) *Service {
	return &Service{mode: mode, repo: repo}
}

func (s *Service) Reserve(ctx context.Context, request domain.ReservationRequest) (*domain.Reservation, error) {
	if !validReservation(request) {
		return nil, fmt.Errorf("invalid inventory reservation")
	}
	return s.repo.Reserve(ctx, request)
}

func (s *Service) ReserveBatch(ctx context.Context, requests []domain.ReservationRequest) ([]domain.Reservation, error) {
	if len(requests) == 0 {
		return nil, fmt.Errorf("inventory reservation batch is empty")
	}
	for _, request := range requests {
		if !validReservation(request) {
			return nil, fmt.Errorf("invalid inventory reservation")
		}
	}
	return s.repo.ReserveBatch(ctx, requests)
}

func validReservation(request domain.ReservationRequest) bool {
	return request.IdempotencyKey != uuid.Nil && request.VariantID != uuid.Nil && request.WarehouseID != uuid.Nil && request.Quantity > 0 && request.ExpiresAt.After(time.Now())
}

func (s *Service) Release(ctx context.Context, reservationID uuid.UUID) error {
	if reservationID == uuid.Nil {
		return fmt.Errorf("invalid inventory reservation id")
	}
	return s.repo.Release(ctx, reservationID)
}

func (s *Service) Commit(ctx context.Context, reservationID, orderID uuid.UUID) error {
	if reservationID == uuid.Nil || orderID == uuid.Nil {
		return fmt.Errorf("invalid inventory commit")
	}
	return s.repo.Commit(ctx, reservationID, orderID)
}

func (s *Service) Adjust(ctx context.Context, variantID, warehouseID uuid.UUID, delta int) error {
	if s.mode != domain.ModeInternal {
		return domain.ErrStockReadOnly
	}
	if variantID == uuid.Nil || warehouseID == uuid.Nil || delta == 0 {
		return fmt.Errorf("invalid stock adjustment")
	}
	return s.repo.Adjust(ctx, variantID, warehouseID, delta)
}

// ReplaceExternalQuantity applies an authoritative ERP snapshot. It is not a
// general-purpose stock edit: internal mode rejects it and the repository
// refuses a quantity lower than active local reservations.
func (s *Service) ReplaceExternalQuantity(ctx context.Context, variantID, warehouseID uuid.UUID, quantity int) error {
	if s.mode != domain.ModeExternal {
		return domain.ErrExternalImportDisabled
	}
	if variantID == uuid.Nil || warehouseID == uuid.Nil || quantity < 0 {
		return fmt.Errorf("invalid external stock quantity")
	}
	return s.repo.ReplaceQuantity(ctx, variantID, warehouseID, quantity)
}

var _ domain.ExternalStockImporter = (*Service)(nil)
