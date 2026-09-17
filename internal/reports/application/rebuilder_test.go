package application

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
	reports "github.com/VladHrytsaiuk/ecommerce-core/internal/reports/domain"
)

func TestReportsRebuilderReplacesRangeAtomically(t *testing.T) {
	t.Parallel()
	paidAt := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	refundedAt := time.Date(2026, 1, 3, 12, 0, 0, 0, time.UTC)
	productID := uuid.New()
	repository := &rebuildRepositoryFake{}
	provider := rebuildSnapshotFake{
		snapshots: []reports.OrderAnalyticsSnapshot{{
			OrderID: uuid.New(), PaidEventID: uuidPtr(uuid.New()), RefundedEventID: uuidPtr(uuid.New()), PaidAt: &paidAt, RefundedAt: &refundedAt, Currency: "UAH", Channel: "web", TotalMinor: 1_000,
			PurchasedProductItems: []reports.PurchasedProductItem{{ProductID: &productID, Quantity: 1, TotalMinor: 1_000, Currency: "UAH"}},
		}},
		funnel: []reports.FunnelEventSnapshot{
			{EventID: uuid.New(), Topic: events.TopicCartCreated, OccurredAt: paidAt, Channel: "web"},
			{EventID: uuid.New(), Topic: events.TopicCheckoutStarted, OccurredAt: paidAt, Channel: "web"},
			{EventID: uuid.New(), Topic: events.TopicOrderPaid, OccurredAt: paidAt, Channel: "web"},
		},
	}
	rebuilder, err := NewReportsRebuilder(repository, rebuildTransactionFake{}, provider, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	period := reports.DateRange{From: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)}
	if err := rebuilder.Rebuild(context.Background(), period); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}
	if !repository.locked || !repository.deleted || len(repository.sales) != 2 || len(repository.products) != 2 || len(repository.funnel) != 3 {
		t.Fatalf("rebuild writes lock=%t deleted=%t sales=%+v products=%+v funnel=%+v", repository.locked, repository.deleted, repository.sales, repository.products, repository.funnel)
	}
	if repository.sales[1].RefundRevenueMinor != 1_000 || repository.sales[1].NetRevenueMinor != -1_000 {
		t.Fatalf("refund aggregate = %+v", repository.sales[1])
	}
}

func TestReportsRebuilderRecoversAsyncPanic(t *testing.T) {
	t.Parallel()
	rebuilder, err := NewReportsRebuilder(&rebuildRepositoryFake{}, rebuildTransactionFake{}, panicSnapshotFake{}, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	started, err := rebuilder.StartAsync(reports.DateRange{From: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)})
	if err != nil || !started {
		t.Fatalf("StartAsync() = %t, %v", started, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := rebuilder.Wait(ctx); err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
	health, err := rebuilder.Health(context.Background())
	if err != nil || health.RebuildActive {
		t.Fatalf("health = %+v, %v", health, err)
	}
}

type rebuildRepositoryFake struct {
	locked, deleted bool
	sales           []reports.DailySales
	products        []reports.DailyProductSales
	funnel          []reports.DailyFunnel
}

func (r *rebuildRepositoryFake) AcquireProjectionLock(context.Context) error {
	r.locked = true
	return nil
}
func (r *rebuildRepositoryFake) DeleteAggregates(context.Context, reports.DateRange, string) error {
	r.deleted = true
	return nil
}
func (r *rebuildRepositoryFake) UpsertDailySales(_ context.Context, value reports.DailySales) error {
	r.sales = append(r.sales, value)
	return nil
}
func (r *rebuildRepositoryFake) UpsertDailyProductSales(_ context.Context, value reports.DailyProductSales) error {
	r.products = append(r.products, value)
	return nil
}
func (r *rebuildRepositoryFake) UpsertDailyFunnel(_ context.Context, value reports.DailyFunnel) error {
	r.funnel = append(r.funnel, value)
	return nil
}
func (*rebuildRepositoryFake) MarkEventProcessed(context.Context, uuid.UUID, string) (bool, error) {
	return false, nil
}
func (*rebuildRepositoryFake) IsEventProcessed(context.Context, uuid.UUID) (bool, error) {
	return false, nil
}
func (*rebuildRepositoryFake) Revenue(context.Context, reports.DateRange, string, string) (reports.RevenueSummary, error) {
	return reports.RevenueSummary{}, nil
}
func (*rebuildRepositoryFake) TopProducts(context.Context, reports.DateRange, string, int) ([]reports.TopProductSales, error) {
	return nil, nil
}
func (*rebuildRepositoryFake) Funnel(context.Context, reports.DateRange) ([]reports.DailyFunnel, error) {
	return nil, nil
}
func (*rebuildRepositoryFake) LastProcessedAt(context.Context) (*time.Time, error) { return nil, nil }

type rebuildTransactionFake struct{}

func (rebuildTransactionFake) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type rebuildSnapshotFake struct {
	snapshots []reports.OrderAnalyticsSnapshot
	funnel    []reports.FunnelEventSnapshot
}

type panicSnapshotFake struct{ rebuildSnapshotFake }

func (panicSnapshotFake) GetSnapshotsByDateRange(context.Context, time.Time, time.Time) ([]reports.OrderAnalyticsSnapshot, error) {
	panic("unexpected source corruption")
}

func (f rebuildSnapshotFake) GetAnalyticsSnapshot(context.Context, uuid.UUID) (reports.OrderAnalyticsSnapshot, error) {
	return reports.OrderAnalyticsSnapshot{}, nil
}
func (f rebuildSnapshotFake) GetSnapshotsByDateRange(context.Context, time.Time, time.Time) ([]reports.OrderAnalyticsSnapshot, error) {
	return f.snapshots, nil
}
func (f rebuildSnapshotFake) GetFunnelEventsByDateRange(context.Context, time.Time, time.Time) ([]reports.FunnelEventSnapshot, error) {
	return f.funnel, nil
}

var _ reports.ReportsRepository = (*rebuildRepositoryFake)(nil)
var _ reports.OrderAnalyticsSnapshotProvider = rebuildSnapshotFake{}

func uuidPtr(value uuid.UUID) *uuid.UUID { return &value }
