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

// activeTrackingStatuses are the delivery statuses still worth asking a carrier
// about. The rest — delivered, failed, cancelled — are terminal.
//
// This list used to read {created, in_transit, shipped, delivered}. 'shipped'
// is not one of this column's statuses at all, so it matched nothing, and
// 'delivered' is terminal, so every completed shipment stayed in the set
// forever and eventually filled the batch to the exclusion of everything in
// transit.
var activeTrackingStatuses = []string{"pending", "created", "in_transit"}

// ClaimDue takes the deliveries whose next check is due, oldest first, and
// defers them by interval inside the same transaction.
//
// The deferral is what makes this safe to run from more than one replica: a row
// this call returns is not due again until the interval elapses, and
// SKIP LOCKED keeps two replicas from contending for the same rows rather than
// one waiting on the other.
func (s *TrackingStore) ClaimDue(ctx context.Context, limit int, now time.Time, interval time.Duration) ([]domain.TrackingDelivery, error) {
	if limit <= 0 {
		limit = 100
	}
	if interval <= 0 {
		return nil, fmt.Errorf("delivery tracking interval must be positive")
	}
	var claimed []domain.TrackingDelivery
	err := transaction.Within(ctx, s.db, func(tx *gorm.DB) error {
		var rows []trackingRow
		if err := tx.Raw(`
			SELECT d.id, d.order_id, d.provider, d.tracking_number, d.status, od.recipient_phone
			FROM deliveries AS d
			JOIN order_delivery_details AS od ON od.order_id = d.order_id
			WHERE d.tracking_number IS NOT NULL
			  AND d.status IN ?
			  AND d.next_check_at <= ?
			ORDER BY d.next_check_at, d.id
			LIMIT ?
			FOR UPDATE OF d SKIP LOCKED`, activeTrackingStatuses, now.UTC(), limit).Scan(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		ids := make([]uuid.UUID, 0, len(rows))
		claimed = make([]domain.TrackingDelivery, 0, len(rows))
		for _, row := range rows {
			ids = append(ids, row.ID)
			claimed = append(claimed, domain.TrackingDelivery{ID: row.ID, OrderID: row.OrderID, Provider: row.Provider, TrackingNumber: row.TrackingNumber, RecipientPhone: row.RecipientPhone, Status: row.Status})
		}
		// Deferred before the carrier is called, not after: a call that hangs
		// or fails must not hand the same row straight back on the next tick.
		return tx.Exec(`UPDATE deliveries SET next_check_at = ? WHERE id IN ?`, now.UTC().Add(interval), ids).Error
	})
	if err != nil {
		return nil, err
	}
	return claimed, nil
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
