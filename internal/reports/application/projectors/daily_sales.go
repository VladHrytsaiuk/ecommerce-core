// Package projectors turns durable domain events into Reports CQRS aggregates.
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

const defaultChannel = "storefront"

// DailySalesProjector is safe for at-least-once delivery: the event marker and
// aggregate increment are committed atomically in one Reports transaction.
type DailySalesProjector struct {
	repository   reports.ReportsRepository
	transactions reports.TransactionManager
	snapshots    reports.OrderAnalyticsSnapshotProvider
	location     *time.Location
	timezone     string
}

func NewDailySalesProjector(repository reports.ReportsRepository, transactions reports.TransactionManager, snapshots reports.OrderAnalyticsSnapshotProvider, timezone string) (*DailySalesProjector, error) {
	if repository == nil || transactions == nil || snapshots == nil {
		return nil, fmt.Errorf("reports repository, transaction manager, and order snapshot provider are required")
	}
	timezone = strings.TrimSpace(timezone)
	if timezone == "" {
		timezone = "UTC"
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, fmt.Errorf("load reports timezone: %w", err)
	}
	return &DailySalesProjector{repository: repository, transactions: transactions, snapshots: snapshots, location: location, timezone: timezone}, nil
}

func (*DailySalesProjector) Topic() string { return events.TopicOrderPaid }

func (p *DailySalesProjector) Handle(ctx context.Context, delivery events.Delivery) error {
	if delivery.EventID == uuid.Nil || delivery.AggregateID == uuid.Nil || delivery.Topic != events.TopicOrderPaid {
		return fmt.Errorf("invalid orders paid delivery")
	}
	var payload struct {
		Version int       `json:"version"`
		OrderID uuid.UUID `json:"order_id"`
	}
	if err := json.Unmarshal(delivery.Payload, &payload); err != nil || payload.Version != 1 || payload.OrderID == uuid.Nil || payload.OrderID != delivery.AggregateID {
		return fmt.Errorf("invalid orders paid event payload")
	}
	snapshot, err := p.snapshots.GetAnalyticsSnapshot(ctx, payload.OrderID)
	if err != nil {
		return fmt.Errorf("load order analytics snapshot: %w", err)
	}
	sales, err := p.salesFromSnapshot(snapshot)
	if err != nil {
		return err
	}
	return p.transactions.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := p.repository.AcquireProjectionLock(txCtx); err != nil {
			return err
		}
		inserted, err := p.repository.MarkEventProcessed(txCtx, delivery.EventID, delivery.Topic)
		if err != nil || !inserted {
			return err
		}
		if err := p.repository.UpsertDailySales(txCtx, sales); err != nil {
			return err
		}
		for _, item := range snapshot.PurchasedProductItems {
			productSales, err := dailyProductSales(sales, item)
			if err != nil {
				return err
			}
			if err := p.repository.UpsertDailyProductSales(txCtx, productSales); err != nil {
				return err
			}
		}
		return p.repository.UpsertDailyFunnel(txCtx, reports.DailyFunnel{BucketDate: sales.BucketDate, Channel: sales.Channel, OrdersPaid: 1})
	})
}

func (p *DailySalesProjector) salesFromSnapshot(snapshot reports.OrderAnalyticsSnapshot) (reports.DailySales, error) {
	if snapshot.OrderID == uuid.Nil || snapshot.PaidAt == nil || strings.TrimSpace(snapshot.Currency) == "" || snapshot.TotalMinor < 0 {
		return reports.DailySales{}, fmt.Errorf("invalid paid order analytics snapshot")
	}
	local := snapshot.PaidAt.In(p.location)
	bucketDate := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, p.location)
	channel := strings.TrimSpace(snapshot.Channel)
	if channel == "" {
		channel = defaultChannel
	}
	return reports.DailySales{
		BucketDate:        bucketDate,
		Timezone:          p.timezone,
		Currency:          strings.ToUpper(strings.TrimSpace(snapshot.Currency)),
		Channel:           channel,
		PaidOrdersCount:   1,
		GrossRevenueMinor: snapshot.TotalMinor,
		NetRevenueMinor:   snapshot.TotalMinor,
	}, nil
}

func dailyProductSales(sales reports.DailySales, item reports.PurchasedProductItem) (reports.DailyProductSales, error) {
	if item.ProductID == nil || *item.ProductID == uuid.Nil || item.Quantity <= 0 || item.TotalMinor < 0 || !strings.EqualFold(strings.TrimSpace(item.Currency), sales.Currency) {
		return reports.DailyProductSales{}, fmt.Errorf("invalid paid order product analytics snapshot")
	}
	return reports.DailyProductSales{
		BucketDate: sales.BucketDate, Currency: sales.Currency, ProductID: *item.ProductID, VariantID: item.VariantID,
		UnitsSold: int64(item.Quantity), GrossRevenueMinor: item.TotalMinor, NetRevenueMinor: item.TotalMinor,
	}, nil
}

var _ interface {
	Topic() string
	Handle(context.Context, events.Delivery) error
} = (*DailySalesProjector)(nil)
