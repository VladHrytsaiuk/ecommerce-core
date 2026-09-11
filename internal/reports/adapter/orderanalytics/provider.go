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
	var header orderHeaderRow
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
LIMIT 1`, events.TopicOrderPaid, orderID).Scan(&header)
	if result.Error != nil {
		return reports.OrderAnalyticsSnapshot{}, result.Error
	}
	if result.RowsAffected != 1 || header.OrderID == uuid.Nil || header.PaidAt.IsZero() {
		return reports.OrderAnalyticsSnapshot{}, fmt.Errorf("order analytics snapshot not found")
	}
	items, err := p.itemsForOrders(ctx, []uuid.UUID{orderID})
	if err != nil {
		return reports.OrderAnalyticsSnapshot{}, err
	}
	return header.toSnapshot(items[orderID]), nil
}

// GetSnapshotsByDateRange returns immutable order facts whose paid or refund
// event falls in the supplied half-open source window. The Reports use case
// converts those timestamps to configured daily buckets; this adapter only
// reads Core snapshots and never exposes Orders models.
//
// It reads the whole window in three queries rather than three per order. The
// previous version looped over the order ids calling GetAnalyticsSnapshot and
// then querying the refund event, which made a month of five thousand orders
// fifteen thousand round trips — all inside the single transaction that also
// holds the projection lock.
func (p *Provider) GetSnapshotsByDateRange(ctx context.Context, from, to time.Time) ([]reports.OrderAnalyticsSnapshot, error) {
	if p == nil || p.db == nil || from.IsZero() || to.IsZero() || !to.After(from) {
		return nil, fmt.Errorf("invalid order analytics rebuild range")
	}
	// DISTINCT ON keeps the latest paid event per order, which is what the
	// per-order query expressed as ORDER BY ... DESC LIMIT 1.
	var headers []orderHeaderRow
	if err := p.database(ctx).Raw(`
SELECT DISTINCT ON (o.id)
       o.id AS order_id, paid.id AS paid_event_id, o.cart_id, o.created_at,
       paid.occurred_at AS paid_at, o.currency,
       o.subtotal_amount AS subtotal_minor,
       o.tax_amount AS tax_minor,
       o.shipping_amount AS shipping_minor,
       o.discount_amount AS discount_minor,
       o.total_amount AS total_minor
FROM orders AS o
JOIN domain_events AS paid
  ON paid.aggregate_id = o.id AND paid.topic = ?
WHERE EXISTS (
    SELECT 1 FROM domain_events e
    WHERE e.aggregate_id = o.id
      AND e.topic IN (?, ?)
      AND e.occurred_at >= ? AND e.occurred_at < ?
)
ORDER BY o.id ASC, paid.occurred_at DESC`,
		events.TopicOrderPaid, events.TopicOrderPaid, events.TopicOrderRefunded, from, to).Scan(&headers).Error; err != nil {
		return nil, err
	}
	if len(headers) == 0 {
		return []reports.OrderAnalyticsSnapshot{}, nil
	}

	orderIDs := make([]uuid.UUID, 0, len(headers))
	for _, header := range headers {
		if header.OrderID == uuid.Nil || header.PaidAt.IsZero() {
			return nil, fmt.Errorf("order analytics snapshot not found")
		}
		orderIDs = append(orderIDs, header.OrderID)
	}

	itemsByOrder, err := p.itemsForOrders(ctx, orderIDs)
	if err != nil {
		return nil, err
	}
	refundsByOrder, err := p.latestRefundForOrders(ctx, orderIDs)
	if err != nil {
		return nil, err
	}

	result := make([]reports.OrderAnalyticsSnapshot, 0, len(headers))
	for index := range headers {
		snapshot := headers[index].toSnapshot(itemsByOrder[headers[index].OrderID])
		if refund, refunded := refundsByOrder[headers[index].OrderID]; refunded {
			occurredAt := refund.OccurredAt
			eventID := refund.EventID
			snapshot.RefundedAt = &occurredAt
			snapshot.RefundedEventID = &eventID
		}
		result = append(result, snapshot)
	}
	return result, nil
}

func (p *Provider) itemsForOrders(ctx context.Context, orderIDs []uuid.UUID) (map[uuid.UUID][]reports.PurchasedProductItem, error) {
	var rows []struct {
		OrderID    uuid.UUID
		ProductID  *uuid.UUID
		VariantID  *uuid.UUID
		Quantity   int
		UnitMinor  int64
		TotalMinor int64
		Currency   string
	}
	if err := p.database(ctx).Raw(`
SELECT item.order_id, variant.product_id, item.variant_id, item.quantity,
       item.unit_price_amount AS unit_minor,
       item.total_amount AS total_minor,
       item.currency
FROM order_items AS item
LEFT JOIN product_variants AS variant ON variant.id = item.variant_id
WHERE item.order_id IN ?
ORDER BY item.order_id, item.id`, orderIDs).Scan(&rows).Error; err != nil {
		return nil, err
	}
	items := make(map[uuid.UUID][]reports.PurchasedProductItem, len(orderIDs))
	for _, row := range rows {
		items[row.OrderID] = append(items[row.OrderID], reports.PurchasedProductItem{
			ProductID: row.ProductID, VariantID: row.VariantID, Quantity: row.Quantity,
			UnitMinor: row.UnitMinor, TotalMinor: row.TotalMinor, Currency: row.Currency,
		})
	}
	return items, nil
}

type refundEventRow struct {
	AggregateID uuid.UUID
	EventID     uuid.UUID
	OccurredAt  time.Time
}

func (p *Provider) latestRefundForOrders(ctx context.Context, orderIDs []uuid.UUID) (map[uuid.UUID]refundEventRow, error) {
	var rows []refundEventRow
	if err := p.database(ctx).Raw(`
SELECT DISTINCT ON (aggregate_id) aggregate_id, id AS event_id, occurred_at
FROM domain_events
WHERE aggregate_id IN ? AND topic = ?
ORDER BY aggregate_id, occurred_at DESC`, orderIDs, events.TopicOrderRefunded).Scan(&rows).Error; err != nil {
		return nil, err
	}
	refunds := make(map[uuid.UUID]refundEventRow, len(rows))
	for _, row := range rows {
		refunds[row.AggregateID] = row
	}
	return refunds, nil
}

// orderHeaderRow is the shape both the single-order and the range query read,
// so the two cannot drift in what an analytics snapshot is made of.
type orderHeaderRow struct {
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

func (r orderHeaderRow) toSnapshot(items []reports.PurchasedProductItem) reports.OrderAnalyticsSnapshot {
	paidEventID, paidAt := r.PaidEventID, r.PaidAt
	if items == nil {
		items = []reports.PurchasedProductItem{}
	}
	return reports.OrderAnalyticsSnapshot{
		OrderID: r.OrderID, CartID: r.CartID, PaidEventID: &paidEventID, CreatedAt: r.CreatedAt, PaidAt: &paidAt,
		Currency: r.Currency, Channel: "storefront", SubtotalMinor: r.SubtotalMinor, TaxMinor: r.TaxMinor,
		ShippingMinor: r.ShippingMinor, DiscountMinor: r.DiscountMinor, TotalMinor: r.TotalMinor,
		PurchasedProductItems: items,
	}
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
