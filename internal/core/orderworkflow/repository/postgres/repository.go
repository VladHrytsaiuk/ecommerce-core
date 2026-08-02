package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	workflowDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/core/orderworkflow/domain"
	ordersDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/orders/domain"
	ordersPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/orders/repository/postgres"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

type reservationRecord struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey"`
	VariantID   uuid.UUID
	WarehouseID uuid.UUID
	Quantity    int
	Status      string
	ExpiresAt   time.Time
	OrderID     *uuid.UUID
}

func (reservationRecord) TableName() string {
	return "inventory_reservations"
}

type orderStateRecord struct {
	ID     uuid.UUID `gorm:"type:uuid;primaryKey"`
	Status string
}

func (orderStateRecord) TableName() string {
	return "orders"
}

func (r *Repository) CreatePending(ctx context.Context, order *ordersDomain.Order, reservationIDs []uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		reservations, err := lockReservations(tx, reservationIDs)
		if err != nil {
			return err
		}
		for _, reservation := range reservations {
			if reservation.Status != "active" || reservation.OrderID != nil || !reservation.ExpiresAt.After(time.Now()) {
				return workflowDomain.ErrReservationUnavailable
			}
		}
		if err := ordersPostgres.CreateInTransaction(tx, order); err != nil {
			return err
		}
		result := tx.Model(&reservationRecord{}).
			Where("id IN ? AND order_id IS NULL AND status = ?", reservationIDs, "active").
			Updates(map[string]any{"order_id": order.ID, "updated_at": gorm.Expr("CURRENT_TIMESTAMP")})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != int64(len(reservationIDs)) {
			return workflowDomain.ErrReservationUnavailable
		}
		return nil
	})
}

func (r *Repository) CancelPending(ctx context.Context, orderID uuid.UUID) error {
	return r.transition(ctx, orderID, ordersDomain.StatusCancelled)
}

func (r *Repository) MarkPaid(ctx context.Context, orderID uuid.UUID) error {
	return r.transition(ctx, orderID, ordersDomain.StatusPaid)
}

func (r *Repository) transition(ctx context.Context, orderID uuid.UUID, targetStatus string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var order orderStateRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&order, "id = ?", orderID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return workflowDomain.ErrInvalidOrderTransition
			}
			return err
		}
		if order.Status == targetStatus {
			return nil
		}
		if order.Status != ordersDomain.StatusPendingPayment {
			return workflowDomain.ErrInvalidOrderTransition
		}

		var reservations []reservationRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("order_id = ?", orderID).Find(&reservations).Error; err != nil {
			return err
		}
		if len(reservations) == 0 {
			return workflowDomain.ErrReservationUnavailable
		}
		for _, reservation := range reservations {
			if reservation.Status != "active" {
				return workflowDomain.ErrReservationUnavailable
			}
			var result *gorm.DB
			if targetStatus == ordersDomain.StatusPaid {
				result = tx.Exec(`UPDATE stock_items SET quantity_on_hand = quantity_on_hand - ?, quantity_reserved = quantity_reserved - ?, updated_at = CURRENT_TIMESTAMP WHERE variant_id = ? AND warehouse_id = ? AND quantity_reserved >= ?`, reservation.Quantity, reservation.Quantity, reservation.VariantID, reservation.WarehouseID, reservation.Quantity)
			} else {
				result = tx.Exec(`UPDATE stock_items SET quantity_reserved = quantity_reserved - ?, updated_at = CURRENT_TIMESTAMP WHERE variant_id = ? AND warehouse_id = ? AND quantity_reserved >= ?`, reservation.Quantity, reservation.VariantID, reservation.WarehouseID, reservation.Quantity)
			}
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return workflowDomain.ErrReservationUnavailable
			}
		}
		reservationStatus := "released"
		if targetStatus == ordersDomain.StatusPaid {
			reservationStatus = "committed"
		}
		if err := tx.Model(&reservationRecord{}).Where("order_id = ? AND status = ?", orderID, "active").Updates(map[string]any{"status": reservationStatus, "updated_at": gorm.Expr("CURRENT_TIMESTAMP")}).Error; err != nil {
			return err
		}
		return tx.Model(&order).Update("status", targetStatus).Error
	})
}

func lockReservations(tx *gorm.DB, reservationIDs []uuid.UUID) ([]reservationRecord, error) {
	query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id IN ?", reservationIDs)
	var reservations []reservationRecord
	if err := query.Find(&reservations).Error; err != nil {
		return nil, err
	}
	if len(reservations) != len(reservationIDs) {
		return nil, workflowDomain.ErrReservationUnavailable
	}
	return reservations, nil
}

var _ workflowDomain.Repository = (*Repository)(nil)
