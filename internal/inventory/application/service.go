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
	if request.IdempotencyKey == uuid.Nil || request.VariantID == uuid.Nil || request.WarehouseID == uuid.Nil || request.Quantity <= 0 || !request.ExpiresAt.After(time.Now()) {
		return nil, fmt.Errorf("invalid inventory reservation")
	}
	return s.repo.Reserve(ctx, request)
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
