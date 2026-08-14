package application

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
	reports "github.com/VladHrytsaiuk/ecommerce-core/internal/reports/domain"
)

const (
	maxRebuildDays     = 366
	maxRebuildDuration = 30 * time.Minute
)

// ReportsRebuilder replaces a bounded reporting window from immutable facts.
// It deliberately has no HTTP dependency; the handler only requests StartAsync.
type ReportsRebuilder struct {
	repository reports.ReportsRepository
	tx         reports.TransactionManager
	snapshots  reports.OrderAnalyticsSnapshotProvider
	timezone   string
	location   *time.Location

	mu        sync.Mutex
	active    bool
	lifecycle context.Context
	wg        sync.WaitGroup
}

func NewReportsRebuilder(repository reports.ReportsRepository, tx reports.TransactionManager, snapshots reports.OrderAnalyticsSnapshotProvider, timezone string) (*ReportsRebuilder, error) {
	if repository == nil || tx == nil || snapshots == nil {
		return nil, fmt.Errorf("reports rebuild dependencies are required")
	}
	timezone = strings.TrimSpace(timezone)
	if timezone == "" {
		timezone = "UTC"
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, fmt.Errorf("load reports timezone: %w", err)
	}
	return &ReportsRebuilder{repository: repository, tx: tx, snapshots: snapshots, timezone: timezone, location: location, lifecycle: context.Background()}, nil
}

// BindLifecycle attaches the Application worker context. A shutdown cancels an
// in-flight rebuild and lets its SQL transaction roll back rather than leaving
// a partially rebuilt range visible.
func (r *ReportsRebuilder) BindLifecycle(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	r.mu.Lock()
	r.lifecycle = ctx
	r.mu.Unlock()
}

func (r *ReportsRebuilder) StartAsync(period reports.DateRange) (bool, error) {
	if err := r.validate(period); err != nil {
		return false, err
	}
	r.mu.Lock()
	if r.active {
		r.mu.Unlock()
		return false, nil
	}
	r.active = true
	ctx := r.lifecycle
	r.wg.Add(1)
	r.mu.Unlock()
	go func() {
		defer r.wg.Done()
		defer func() {
			r.mu.Lock()
			r.active = false
			r.mu.Unlock()
		}()
		// This context is deliberately derived from the application lifecycle,
		// never the HTTP request. Client disconnects cannot interrupt the SQL
		// transaction; application shutdown and this hard ceiling can.
		rebuildCtx, cancel := context.WithTimeout(ctx, maxRebuildDuration)
		defer cancel()
		defer func() { _ = recover() }()
		_ = r.Rebuild(rebuildCtx, period)
	}()
	return true, nil
}

// Rebuild synchronously rebuilds a date-only reporting window. It is exposed
// for CLI/tests; production HTTP uses StartAsync to keep the request bounded.
func (r *ReportsRebuilder) Rebuild(ctx context.Context, period reports.DateRange) error {
	if err := r.validate(period); err != nil {
		return err
	}
	return r.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := r.repository.AcquireProjectionLock(txCtx); err != nil {
			return err
		}
		if err := r.repository.DeleteAggregates(txCtx, period, r.timezone); err != nil {
			return err
		}
		from, to := r.sourceBounds(period)
		snapshots, err := r.snapshots.GetSnapshotsByDateRange(txCtx, from, to)
		if err != nil {
			return err
		}
		for _, snapshot := range snapshots {
			if err := r.upsertOrder(txCtx, period, snapshot); err != nil {
				return err
			}
		}
		funnelEvents, err := r.snapshots.GetFunnelEventsByDateRange(txCtx, from, to)
		if err != nil {
			return err
		}
		for _, event := range funnelEvents {
			if err := r.upsertFunnelEvent(txCtx, period, event); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *ReportsRebuilder) Health(ctx context.Context) (reports.Health, error) {
	lastProcessed, err := r.repository.LastProcessedAt(ctx)
	if err != nil {
		return reports.Health{}, err
	}
	r.mu.Lock()
	active := r.active
	r.mu.Unlock()
	return reports.Health{RebuildActive: active, LastEventProcessedAt: lastProcessed}, nil
}

func (r *ReportsRebuilder) Wait(ctx context.Context) error {
	done := make(chan struct{})
	go func() { r.wg.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *ReportsRebuilder) upsertOrder(ctx context.Context, period reports.DateRange, snapshot reports.OrderAnalyticsSnapshot) error {
	if snapshot.OrderID == uuid.Nil || strings.TrimSpace(snapshot.Currency) == "" {
		return fmt.Errorf("invalid rebuild order snapshot")
	}
	channel := strings.TrimSpace(snapshot.Channel)
	if channel == "" {
		channel = "storefront"
	}
	currency := strings.ToUpper(strings.TrimSpace(snapshot.Currency))
	if snapshot.PaidAt != nil && r.inRange(period, *snapshot.PaidAt) {
		bucket := r.bucket(*snapshot.PaidAt)
		if err := r.repository.UpsertDailySales(ctx, reports.DailySales{BucketDate: bucket, Timezone: r.timezone, Currency: currency, Channel: channel, PaidOrdersCount: 1, GrossRevenueMinor: snapshot.TotalMinor, NetRevenueMinor: snapshot.TotalMinor}); err != nil {
			return err
		}
		for _, item := range snapshot.PurchasedProductItems {
			if err := r.upsertProduct(ctx, bucket, currency, item, 1); err != nil {
				return err
			}
		}
		if err := r.markRebuiltEvent(ctx, snapshot.PaidEventID, events.TopicOrderPaid); err != nil {
			return err
		}
	}
	if snapshot.RefundedAt != nil && r.inRange(period, *snapshot.RefundedAt) {
		bucket := r.bucket(*snapshot.RefundedAt)
		if err := r.repository.UpsertDailySales(ctx, reports.DailySales{BucketDate: bucket, Timezone: r.timezone, Currency: currency, Channel: channel, RefundedOrdersCount: 1, RefundRevenueMinor: snapshot.TotalMinor, NetRevenueMinor: -snapshot.TotalMinor}); err != nil {
			return err
		}
		for _, item := range snapshot.PurchasedProductItems {
			if err := r.upsertProduct(ctx, bucket, currency, item, -1); err != nil {
				return err
			}
		}
		if err := r.markRebuiltEvent(ctx, snapshot.RefundedEventID, events.TopicOrderRefunded); err != nil {
			return err
		}
	}
	return nil
}

func (r *ReportsRebuilder) upsertProduct(ctx context.Context, bucket time.Time, currency string, item reports.PurchasedProductItem, direction int64) error {
	if item.ProductID == nil || *item.ProductID == uuid.Nil || item.Quantity <= 0 || item.TotalMinor < 0 || !strings.EqualFold(item.Currency, currency) {
		return fmt.Errorf("invalid rebuild product snapshot")
	}
	sales := reports.DailyProductSales{BucketDate: bucket, Currency: currency, ProductID: *item.ProductID, VariantID: item.VariantID}
	if direction > 0 {
		sales.UnitsSold, sales.GrossRevenueMinor, sales.NetRevenueMinor = int64(item.Quantity), item.TotalMinor, item.TotalMinor
	} else {
		sales.NetRevenueMinor = -item.TotalMinor
	}
	return r.repository.UpsertDailyProductSales(ctx, sales)
}

func (r *ReportsRebuilder) upsertFunnelEvent(ctx context.Context, period reports.DateRange, event reports.FunnelEventSnapshot) error {
	if event.EventID == uuid.Nil || event.OccurredAt.IsZero() || !r.inRange(period, event.OccurredAt) {
		return fmt.Errorf("invalid rebuild funnel event")
	}
	funnel := reports.DailyFunnel{BucketDate: r.bucket(event.OccurredAt), Channel: strings.TrimSpace(event.Channel)}
	if funnel.Channel == "" {
		funnel.Channel = "storefront"
	}
	switch event.Topic {
	case events.TopicCartCreated:
		funnel.CartsCreated = 1
	case events.TopicCheckoutStarted:
		funnel.CheckoutsStarted = 1
	case events.TopicOrderPaid:
		funnel.OrdersPaid = 1
	default:
		return fmt.Errorf("unsupported rebuild funnel event topic")
	}
	if err := r.repository.UpsertDailyFunnel(ctx, funnel); err != nil {
		return err
	}
	// The paid event was already fenced by the sales rebuild above; a duplicate
	// insert is intentionally a no-op. Cart and checkout events are fenced here.
	_, err := r.repository.MarkEventProcessed(ctx, event.EventID, event.Topic)
	return err
}

func (r *ReportsRebuilder) markRebuiltEvent(ctx context.Context, eventID *uuid.UUID, topic string) error {
	if eventID == nil || *eventID == uuid.Nil {
		return fmt.Errorf("rebuild source event ID is required")
	}
	_, err := r.repository.MarkEventProcessed(ctx, *eventID, topic)
	return err
}

func (r *ReportsRebuilder) validate(period reports.DateRange) error {
	if period.From.IsZero() || period.To.IsZero() || period.To.Before(period.From) || period.To.Sub(period.From) > maxRebuildDays*24*time.Hour {
		return fmt.Errorf("reports rebuild range is invalid")
	}
	return nil
}

func (r *ReportsRebuilder) sourceBounds(period reports.DateRange) (time.Time, time.Time) {
	from := time.Date(period.From.Year(), period.From.Month(), period.From.Day(), 0, 0, 0, 0, r.location)
	to := time.Date(period.To.Year(), period.To.Month(), period.To.Day()+1, 0, 0, 0, 0, r.location)
	return from, to
}
func (r *ReportsRebuilder) bucket(value time.Time) time.Time {
	local := value.In(r.location)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, r.location)
}
func (r *ReportsRebuilder) inRange(period reports.DateRange, value time.Time) bool {
	day := r.bucket(value)
	from := time.Date(period.From.Year(), period.From.Month(), period.From.Day(), 0, 0, 0, 0, r.location)
	to := time.Date(period.To.Year(), period.To.Month(), period.To.Day(), 0, 0, 0, 0, r.location)
	return !day.Before(from) && !day.After(to)
}

var _ reports.Rebuilder = (*ReportsRebuilder)(nil)
