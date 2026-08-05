package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/inventory/domain"
)

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

func (r *Repository) Reserve(ctx context.Context, request domain.ReservationRequest) (*domain.Reservation, error) {
	reservations, err := r.ReserveBatch(ctx, []domain.ReservationRequest{request})
	if err != nil {
		return nil, err
	}
	return &reservations[0], nil
}

func (r *Repository) ReserveBatch(ctx context.Context, requests []domain.ReservationRequest) ([]domain.Reservation, error) {
	reservations := make([]domain.Reservation, 0, len(requests))
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, request := range requests {
			reservation, err := reserveInTransaction(tx, request)
			if err != nil {
				return err
			}
			reservations = append(reservations, *reservation)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return reservations, nil
}

func reserveInTransaction(tx *gorm.DB, request domain.ReservationRequest) (*domain.Reservation, error) {
	reservation := &domain.Reservation{ID: uuid.New(), IdempotencyKey: request.IdempotencyKey, VariantID: request.VariantID, WarehouseID: request.WarehouseID, Quantity: request.Quantity, Status: "active", ExpiresAt: request.ExpiresAt}
	result := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "idempotency_key"}}, DoNothing: true}).Create(reservation)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		if err := tx.Where("idempotency_key = ?", request.IdempotencyKey).First(reservation).Error; err != nil {
			return nil, err
		}
		return reservation, nil
	}
	result = tx.Exec(`UPDATE stock_items
        SET quantity_reserved = quantity_reserved + ?, updated_at = CURRENT_TIMESTAMP
        WHERE variant_id = ? AND warehouse_id = ?
          AND quantity_on_hand - quantity_reserved >= ?`, request.Quantity, request.VariantID, request.WarehouseID, request.Quantity)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, domain.ErrInsufficientStock
	}
	return reservation, nil
}

func (r *Repository) Release(ctx context.Context, reservationID uuid.UUID) error {
	return r.transition(ctx, reservationID, nil, "released")
}

func (r *Repository) Commit(ctx context.Context, reservationID, orderID uuid.UUID) error {
	return r.transition(ctx, reservationID, &orderID, "committed")
}

func (r *Repository) transition(ctx context.Context, reservationID uuid.UUID, orderID *uuid.UUID, target string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var reservation domain.Reservation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&reservation, "id = ?", reservationID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrReservationNotFound
			}
			return err
		}
		if reservation.Status != "active" {
			return domain.ErrReservationInactive
		}
		var result *gorm.DB
		if target == "committed" {
			result = tx.Exec(`UPDATE stock_items SET quantity_on_hand = quantity_on_hand - ?, quantity_reserved = quantity_reserved - ?, updated_at = CURRENT_TIMESTAMP WHERE variant_id = ? AND warehouse_id = ? AND quantity_reserved >= ?`, reservation.Quantity, reservation.Quantity, reservation.VariantID, reservation.WarehouseID, reservation.Quantity)
		} else {
			result = tx.Exec(`UPDATE stock_items SET quantity_reserved = quantity_reserved - ?, updated_at = CURRENT_TIMESTAMP WHERE variant_id = ? AND warehouse_id = ? AND quantity_reserved >= ?`, reservation.Quantity, reservation.VariantID, reservation.WarehouseID, reservation.Quantity)
		}
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return domain.ErrInsufficientStock
		}
		return tx.Model(&reservation).Updates(map[string]any{"status": target, "order_id": orderID, "updated_at": gorm.Expr("CURRENT_TIMESTAMP")}).Error
	})
}

func (r *Repository) Adjust(ctx context.Context, variantID, warehouseID uuid.UUID, delta int) error {
	if delta > 0 {
		return r.db.WithContext(ctx).Exec(`INSERT INTO stock_items (id, variant_id, warehouse_id, quantity_on_hand) VALUES (?, ?, ?, ?) ON CONFLICT (variant_id, warehouse_id) DO UPDATE SET quantity_on_hand = stock_items.quantity_on_hand + EXCLUDED.quantity_on_hand, updated_at = CURRENT_TIMESTAMP`, uuid.New(), variantID, warehouseID, delta).Error
	}
	result := r.db.WithContext(ctx).Exec(`UPDATE stock_items SET quantity_on_hand = quantity_on_hand + ?, updated_at = CURRENT_TIMESTAMP WHERE variant_id = ? AND warehouse_id = ? AND quantity_on_hand + ? >= quantity_reserved`, delta, variantID, warehouseID, delta)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrInsufficientStock
	}
	return nil
}

// ReplaceQuantity preserves already promised local stock. An ERP snapshot may
// arrive late, but it may never invalidate an active checkout reservation.
func (r *Repository) ReplaceQuantity(ctx context.Context, variantID, warehouseID uuid.UUID, quantity int) error {
	result := r.db.WithContext(ctx).Exec(`INSERT INTO stock_items (id, variant_id, warehouse_id, quantity_on_hand)
VALUES (?, ?, ?, ?)
ON CONFLICT (variant_id, warehouse_id) DO UPDATE
SET quantity_on_hand = EXCLUDED.quantity_on_hand, updated_at = CURRENT_TIMESTAMP
WHERE stock_items.quantity_reserved <= EXCLUDED.quantity_on_hand`, uuid.New(), variantID, warehouseID, quantity)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return domain.ErrInsufficientStock
	}
	return nil
}

// ReleaseExpiredUnattached only releases reservations that never reached an
// order workflow. Attached pending orders require provider cancellation first.
func (r *Repository) ReleaseExpiredUnattached(ctx context.Context, now time.Time, limit int) (int, error) {
	if limit <= 0 {
		limit = 100
	}
	count := 0
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rows []domain.Reservation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).Where("status = ? AND order_id IS NULL AND expires_at <= ?", "active", now).Order("expires_at").Limit(limit).Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			result := tx.Exec(`UPDATE stock_items SET quantity_reserved = quantity_reserved - ?, updated_at = CURRENT_TIMESTAMP WHERE variant_id = ? AND warehouse_id = ? AND quantity_reserved >= ?`, row.Quantity, row.VariantID, row.WarehouseID, row.Quantity)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return domain.ErrInsufficientStock
			}
			if err := tx.Model(&row).Updates(map[string]any{"status": "expired", "updated_at": gorm.Expr("CURRENT_TIMESTAMP")}).Error; err != nil {
				return err
			}
			count++
		}
		return nil
	})
	return count, err
}
