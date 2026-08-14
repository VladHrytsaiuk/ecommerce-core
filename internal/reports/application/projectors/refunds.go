package projectors

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
	reports "github.com/VladHrytsaiuk/ecommerce-core/internal/reports/domain"
)

// RefundProjector projects verified full refunds. The event idempotency marker
// and all revenue reversals use one local transaction.
type RefundProjector struct {
	repository reports.ReportsRepository
	tx         reports.TransactionManager
	snapshots  reports.OrderAnalyticsSnapshotProvider
	location   *time.Location
	timezone   string
}

func NewRefundProjector(repository reports.ReportsRepository, tx reports.TransactionManager, snapshots reports.OrderAnalyticsSnapshotProvider, timezone string) (*RefundProjector, error) {
	if repository == nil || tx == nil || snapshots == nil {
		return nil, fmt.Errorf("reports refund projector dependencies are required")
	}
	if strings.TrimSpace(timezone) == "" {
		timezone = "UTC"
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, fmt.Errorf("load reports timezone: %w", err)
	}
	return &RefundProjector{repository: repository, tx: tx, snapshots: snapshots, location: location, timezone: timezone}, nil
}
func (*RefundProjector) Topic() string { return events.TopicOrderRefunded }
func (p *RefundProjector) Handle(ctx context.Context, delivery events.Delivery) error {
	var payload struct {
		Version    int       `json:"version"`
		OrderID    uuid.UUID `json:"order_id"`
		Amount     int64     `json:"amount_minor"`
		Currency   string    `json:"currency"`
		RefundedAt time.Time `json:"refunded_at"`
	}
	if delivery.EventID == uuid.Nil || delivery.AggregateID == uuid.Nil || delivery.Topic != events.TopicOrderRefunded || json.Unmarshal(delivery.Payload, &payload) != nil || payload.Version != 1 || payload.OrderID != delivery.AggregateID || payload.Amount < 0 || strings.TrimSpace(payload.Currency) == "" || payload.RefundedAt.IsZero() {
		return fmt.Errorf("invalid orders refunded event")
	}
	snapshot, err := p.snapshots.GetAnalyticsSnapshot(ctx, payload.OrderID)
	if err != nil {
		return fmt.Errorf("load order analytics snapshot: %w", err)
	}
	if snapshot.OrderID != payload.OrderID || snapshot.TotalMinor != payload.Amount || !strings.EqualFold(snapshot.Currency, payload.Currency) {
		return fmt.Errorf("refund analytics snapshot does not match event")
	}
	local := payload.RefundedAt.In(p.location)
	bucket := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, p.location)
	channel := strings.TrimSpace(snapshot.Channel)
	if channel == "" {
		channel = defaultChannel
	}
	return p.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := p.repository.AcquireProjectionLock(txCtx); err != nil {
			return err
		}
		inserted, err := p.repository.MarkEventProcessed(txCtx, delivery.EventID, delivery.Topic)
		if err != nil || !inserted {
			return err
		}
		if err := p.repository.UpsertDailySales(txCtx, reports.DailySales{BucketDate: bucket, Timezone: p.timezone, Currency: strings.ToUpper(payload.Currency), Channel: channel, RefundedOrdersCount: 1, RefundRevenueMinor: payload.Amount, NetRevenueMinor: -payload.Amount}); err != nil {
			return err
		}
		for _, item := range snapshot.PurchasedProductItems {
			if item.ProductID == nil || *item.ProductID == uuid.Nil || item.TotalMinor < 0 || !strings.EqualFold(item.Currency, payload.Currency) {
				return fmt.Errorf("invalid refunded product analytics snapshot")
			}
			if err := p.repository.UpsertDailyProductSales(txCtx, reports.DailyProductSales{BucketDate: bucket, Currency: strings.ToUpper(payload.Currency), ProductID: *item.ProductID, VariantID: item.VariantID, NetRevenueMinor: -item.TotalMinor}); err != nil {
				return err
			}
		}
		return nil
	})
}

var _ interface {
	Topic() string
	Handle(context.Context, events.Delivery) error
} = (*RefundProjector)(nil)
