package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/delivery/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/postgres/transaction"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type TrackingStore struct{ db *gorm.DB }

func NewTrackingStore(db *gorm.DB) *TrackingStore { return &TrackingStore{db: db} }

type trackingRow struct {
	ID, OrderID                              uuid.UUID
	Provider, TrackingNumber, RecipientPhone string
	Status                                   string
}

func (s *TrackingStore) ListActive(ctx context.Context, limit int) ([]domain.TrackingDelivery, error) {
	if limit <= 0 {
		limit = 100
	}
	var rows []trackingRow
	err := s.db.WithContext(ctx).Table("deliveries").Select("deliveries.id, deliveries.order_id, deliveries.provider, deliveries.tracking_number, deliveries.status, order_delivery_details.recipient_phone").Joins("JOIN order_delivery_details ON order_delivery_details.order_id = deliveries.order_id").Where("deliveries.status IN ? AND deliveries.tracking_number IS NOT NULL", []string{"created", "in_transit", "shipped", "delivered"}).Limit(limit).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	result := make([]domain.TrackingDelivery, 0, len(rows))
	for _, row := range rows {
		result = append(result, domain.TrackingDelivery{ID: row.ID, OrderID: row.OrderID, Provider: row.Provider, TrackingNumber: row.TrackingNumber, RecipientPhone: row.RecipientPhone, Status: row.Status})
	}
	return result, nil
}
func (s *TrackingStore) UpdateStatusAndTransition(ctx context.Context, delivery domain.TrackingDelivery, result domain.TrackingResult, orders domain.OrderTransitioner) error {
	if !validDeliveryStatus(result.Status) {
		return fmt.Errorf("invalid delivery tracking status %q", result.Status)
	}
	return transaction.Within(ctx, s.db, func(tx *gorm.DB) error {
		var row trackingRow
		if err := tx.Table("deliveries").Clauses(clause.Locking{Strength: "UPDATE"}).Select("id, order_id, status").First(&row, "id = ?", delivery.ID).Error; err != nil {
			return err
		}
		if row.Status == result.Status {
			return nil
		}
		values := map[string]any{"status": result.Status, "updated_at": gorm.Expr("CURRENT_TIMESTAMP")}
		if !result.OccurredAt.IsZero() {
			values["updated_at"] = result.OccurredAt.UTC()
		}
		if err := tx.Table("deliveries").Where("id = ? AND status = ?", delivery.ID, row.Status).Updates(values).Error; err != nil {
			return err
		}
		if result.OrderStatusCode == "" || orders == nil {
			return nil
		}
		occurredAt := result.OccurredAt
		if occurredAt.IsZero() {
			occurredAt = time.Now().UTC()
		}
		return orders.TransitionFromDelivery(transaction.WithContext(ctx, tx), domain.OrderStatusTransition{DeliveryID: delivery.ID, OrderID: row.OrderID, ToStatusCode: result.OrderStatusCode, TrackingNumber: delivery.TrackingNumber, OccurredAt: occurredAt})
	})
}

func validDeliveryStatus(status string) bool {
	switch status {
	case "shipped", "delivered", "received", "cancelled", "failed", "in_transit":
		return true
	default:
		return false
	}
}

var _ domain.TrackingStore = (*TrackingStore)(nil)
