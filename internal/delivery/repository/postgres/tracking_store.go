package postgres

import (
	"context"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/delivery/domain"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type TrackingStore struct{ db *gorm.DB }

func NewTrackingStore(db *gorm.DB) *TrackingStore { return &TrackingStore{db: db} }

type trackingRow struct {
	ID                                       uuid.UUID
	Provider, TrackingNumber, RecipientPhone string
}

func (s *TrackingStore) ListActive(ctx context.Context, limit int) ([]domain.TrackingDelivery, error) {
	if limit <= 0 {
		limit = 100
	}
	var rows []trackingRow
	err := s.db.WithContext(ctx).Table("deliveries").Select("deliveries.id, deliveries.provider, deliveries.tracking_number, order_delivery_details.recipient_phone").Joins("JOIN order_delivery_details ON order_delivery_details.order_id = deliveries.order_id").Where("deliveries.status IN ? AND deliveries.tracking_number IS NOT NULL", []string{"created", "in_transit"}).Limit(limit).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	result := make([]domain.TrackingDelivery, 0, len(rows))
	for _, row := range rows {
		result = append(result, domain.TrackingDelivery{ID: row.ID, Provider: row.Provider, TrackingNumber: row.TrackingNumber, RecipientPhone: row.RecipientPhone})
	}
	return result, nil
}
func (s *TrackingStore) UpdateStatus(ctx context.Context, id uuid.UUID, result domain.TrackingResult) error {
	if result.Status != "in_transit" && result.Status != "delivered" && result.Status != "failed" {
		return nil
	}
	values := map[string]any{"status": result.Status, "updated_at": gorm.Expr("CURRENT_TIMESTAMP")}
	if !result.OccurredAt.IsZero() {
		values["updated_at"] = result.OccurredAt.UTC()
	}
	return s.db.WithContext(ctx).Table("deliveries").Where("id = ? AND status IN ?", id, []string{"created", "in_transit"}).Updates(values).Error
}

var _ domain.TrackingStore = (*TrackingStore)(nil)
