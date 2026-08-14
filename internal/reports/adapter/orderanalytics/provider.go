// Package orderanalytics adapts stable Core order snapshots to the narrow
// Reports port. Reports domain code never imports Orders models or repositories.
package orderanalytics

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
	transaction "github.com/VladHrytsaiuk/ecommerce-core/internal/platform/postgres/transaction"
	reports "github.com/VladHrytsaiuk/ecommerce-core/internal/reports/domain"
)

type Provider struct{ db *gorm.DB }

func NewProvider(db *gorm.DB) *Provider { return &Provider{db: db} }

func (p *Provider) GetAnalyticsSnapshot(ctx context.Context, orderID uuid.UUID) (reports.OrderAnalyticsSnapshot, error) {
	if p == nil || p.db == nil || orderID == uuid.Nil {
		return reports.OrderAnalyticsSnapshot{}, fmt.Errorf("invalid order analytics snapshot request")
	}
	var order struct {
		OrderID       uuid.UUID
		PaidEventID   uuid.UUID
		CartID        *uuid.UUID
		CreatedAt     time.Time
		PaidAt        time.Time
		Currency      string
		SubtotalMinor int64
		TaxMinor      int64
		ShippingMinor int64
		DiscountMinor int64
		TotalMinor    int64
	}
	result := p.database(ctx).Raw(`
SELECT o.id AS order_id, event.id AS paid_event_id, o.cart_id, o.created_at, event.occurred_at AS paid_at,
       o.currency,
       o.subtotal_amount AS subtotal_minor,
       o.tax_amount AS tax_minor,
       o.shipping_amount AS shipping_minor,
       o.discount_amount AS discount_minor,
       o.total_amount AS total_minor
FROM orders AS o
JOIN domain_events AS event
  ON event.aggregate_id = o.id
 AND event.topic = ?
WHERE o.id = ?
ORDER BY event.occurred_at DESC
LIMIT 1`, events.TopicOrderPaid, orderID).Scan(&order)
	if result.Error != nil {
		return reports.OrderAnalyticsSnapshot{}, result.Error
	}
	if result.RowsAffected != 1 || order.OrderID == uuid.Nil || order.PaidAt.IsZero() {
		return reports.OrderAnalyticsSnapshot{}, fmt.Errorf("order analytics snapshot not found")
	}
	var items []struct {
		ProductID  *uuid.UUID
		VariantID  *uuid.UUID
		Quantity   int
		UnitMinor  int64
		TotalMinor int64
		Currency   string
	}
	if err := p.database(ctx).Raw(`
SELECT variant.product_id, item.variant_id, item.quantity,
       item.unit_price_amount AS unit_minor,
       item.total_amount AS total_minor,
       item.currency
FROM order_items AS item
LEFT JOIN product_variants AS variant ON variant.id = item.variant_id
WHERE item.order_id = ?
ORDER BY item.id`, orderID).Scan(&items).Error; err != nil {
		return reports.OrderAnalyticsSnapshot{}, err
	}
	snapshot := reports.OrderAnalyticsSnapshot{
		OrderID: order.OrderID, CartID: order.CartID, PaidEventID: &order.PaidEventID, CreatedAt: order.CreatedAt, PaidAt: &order.PaidAt,
		Currency: order.Currency, Channel: "storefront", SubtotalMinor: order.SubtotalMinor, TaxMinor: order.TaxMinor,
		ShippingMinor: order.ShippingMinor, DiscountMinor: order.DiscountMinor, TotalMinor: order.TotalMinor,
		PurchasedProductItems: make([]reports.PurchasedProductItem, 0, len(items)),
	}
	for _, item := range items {
		snapshot.PurchasedProductItems = append(snapshot.PurchasedProductItems, reports.PurchasedProductItem{
			ProductID: item.ProductID, VariantID: item.VariantID, Quantity: item.Quantity,
			UnitMinor: item.UnitMinor, TotalMinor: item.TotalMinor, Currency: item.Currency,
		})
	}
	return snapshot, nil
}

// GetSnapshotsByDateRange returns immutable order facts whose paid or refund
// event falls in the supplied half-open source window. The Reports use case
// converts those timestamps to configured daily buckets; this adapter only
// reads Core snapshots and never exposes Orders models.
func (p *Provider) GetSnapshotsByDateRange(ctx context.Context, from, to time.Time) ([]reports.OrderAnalyticsSnapshot, error) {
	if p == nil || p.db == nil || from.IsZero() || to.IsZero() || !to.After(from) {
		return nil, fmt.Errorf("invalid order analytics rebuild range")
	}
	var orderIDs []uuid.UUID
	if err := p.database(ctx).Raw(`
SELECT o.id
FROM orders AS o
WHERE EXISTS (
    SELECT 1 FROM domain_events e
    WHERE e.aggregate_id = o.id
      AND e.topic IN (?, ?)
      AND e.occurred_at >= ? AND e.occurred_at < ?
)
ORDER BY o.id ASC`, events.TopicOrderPaid, events.TopicOrderRefunded, from, to).Scan(&orderIDs).Error; err != nil {
		return nil, err
	}
	result := make([]reports.OrderAnalyticsSnapshot, 0, len(orderIDs))
	for _, orderID := range orderIDs {
		snapshot, err := p.GetAnalyticsSnapshot(ctx, orderID)
		if err != nil {
			return nil, err
		}
		var refundedAt *time.Time
		var row struct {
			EventID    uuid.UUID
			OccurredAt time.Time
		}
		query := p.database(ctx).Raw(`
SELECT id AS event_id, occurred_at FROM domain_events
WHERE aggregate_id = ? AND topic = ?
ORDER BY occurred_at DESC LIMIT 1`, orderID, events.TopicOrderRefunded).Scan(&row)
		if query.Error != nil {
			return nil, query.Error
		}
		if query.RowsAffected == 1 {
			refundedAt = &row.OccurredAt
			snapshot.RefundedEventID = &row.EventID
		}
		snapshot.RefundedAt = refundedAt
		result = append(result, snapshot)
	}
	return result, nil
}

// GetFunnelEventsByDateRange reads only anonymous lifecycle events. It is
// deliberately separate from order snapshots so abandoned carts remain part
// of a funnel rebuild.
func (p *Provider) GetFunnelEventsByDateRange(ctx context.Context, from, to time.Time) ([]reports.FunnelEventSnapshot, error) {
	if p == nil || p.db == nil || from.IsZero() || to.IsZero() || !to.After(from) {
		return nil, fmt.Errorf("invalid funnel analytics rebuild range")
	}
	var rows []reports.FunnelEventSnapshot
	if err := p.database(ctx).Raw(`
SELECT id AS event_id, topic, occurred_at, 'storefront' AS channel
FROM domain_events
WHERE topic IN (?, ?, ?)
  AND occurred_at >= ? AND occurred_at < ?
ORDER BY occurred_at ASC, id ASC`, events.TopicCartCreated, events.TopicCheckoutStarted, events.TopicOrderPaid, from, to).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (p *Provider) database(ctx context.Context) *gorm.DB {
	if tx, err := transaction.FromContext(ctx); err == nil {
		return tx.WithContext(ctx)
	}
	return p.db.WithContext(ctx)
}

var _ reports.OrderAnalyticsSnapshotProvider = (*Provider)(nil)
