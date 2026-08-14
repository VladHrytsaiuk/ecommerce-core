package projectors

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
	reports "github.com/VladHrytsaiuk/ecommerce-core/internal/reports/domain"
)

func TestDailySalesProjectorIsIdempotent(t *testing.T) {
	t.Parallel()
	orderID, eventID := uuid.New(), uuid.New()
	paidAt := time.Date(2026, time.January, 2, 12, 0, 0, 0, time.UTC)
	repository := &fakeRepository{}
	productID := uuid.New()
	projector := newProjector(t, repository, reports.OrderAnalyticsSnapshot{OrderID: orderID, PaidAt: &paidAt, Currency: "uah", TotalMinor: 1_250, PurchasedProductItems: []reports.PurchasedProductItem{{ProductID: &productID, Quantity: 2, TotalMinor: 1_250, Currency: "UAH"}}})
	delivery := paidDelivery(t, eventID, orderID)
	if err := projector.Handle(context.Background(), delivery); err != nil {
		t.Fatalf("first Handle() error = %v", err)
	}
	if err := projector.Handle(context.Background(), delivery); err != nil {
		t.Fatalf("duplicate Handle() error = %v", err)
	}
	if len(repository.sales) != 1 {
		t.Fatalf("UpsertDailySales calls = %d, want 1", len(repository.sales))
	}
	if len(repository.products) != 1 || repository.products[0].UnitsSold != 2 || repository.products[0].GrossRevenueMinor != 1_250 {
		t.Fatalf("product sales = %+v", repository.products)
	}
	got := repository.sales[0]
	if got.PaidOrdersCount != 1 || got.GrossRevenueMinor != 1_250 || got.NetRevenueMinor != 1_250 || got.RefundRevenueMinor != 0 {
		t.Fatalf("daily sales = %+v", got)
	}
}

func TestDailySalesProjectorBucketsPaidTimeInConfiguredTimezone(t *testing.T) {
	t.Parallel()
	orderID := uuid.New()
	// 22:30 UTC is already the next calendar day in Europe/Kyiv in January.
	paidAt := time.Date(2026, time.January, 1, 22, 30, 0, 0, time.UTC)
	repository := &fakeRepository{}
	productID := uuid.New()
	projector := newProjector(t, repository, reports.OrderAnalyticsSnapshot{OrderID: orderID, PaidAt: &paidAt, Currency: "EUR", Channel: "web", TotalMinor: 990, PurchasedProductItems: []reports.PurchasedProductItem{{ProductID: &productID, Quantity: 1, TotalMinor: 990, Currency: "EUR"}}})
	if err := projector.Handle(context.Background(), paidDelivery(t, uuid.New(), orderID)); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	got := repository.sales[0]
	if got.BucketDate.Format("2006-01-02") != "2026-01-02" || got.Timezone != "Europe/Kyiv" || got.Channel != "web" {
		t.Fatalf("bucket = %+v", got)
	}
}

func newProjector(t *testing.T, repository *fakeRepository, snapshot reports.OrderAnalyticsSnapshot) *DailySalesProjector {
	t.Helper()
	projector, err := NewDailySalesProjector(repository, fakeTransactions{}, fakeSnapshots{snapshot: snapshot}, "Europe/Kyiv")
	if err != nil {
		t.Fatalf("NewDailySalesProjector() error = %v", err)
	}
	return projector
}

func paidDelivery(t *testing.T, eventID, orderID uuid.UUID) events.Delivery {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"version": 1, "order_id": orderID})
	if err != nil {
		t.Fatal(err)
	}
	return events.Delivery{EventID: eventID, AggregateID: orderID, Topic: events.TopicOrderPaid, Payload: payload}
}

type fakeRepository struct {
	processed map[uuid.UUID]bool
	sales     []reports.DailySales
	products  []reports.DailyProductSales
	funnels   []reports.DailyFunnel
}

func (r *fakeRepository) UpsertDailyProductSales(_ context.Context, sales reports.DailyProductSales) error {
	r.products = append(r.products, sales)
	return nil
}

func (r *fakeRepository) UpsertDailySales(_ context.Context, sales reports.DailySales) error {
	r.sales = append(r.sales, sales)
	return nil
}
func (r *fakeRepository) UpsertDailyFunnel(_ context.Context, funnel reports.DailyFunnel) error {
	r.funnels = append(r.funnels, funnel)
	return nil
}
func (*fakeRepository) DeleteAggregates(context.Context, reports.DateRange, string) error { return nil }
func (*fakeRepository) AcquireProjectionLock(context.Context) error                       { return nil }
func (r *fakeRepository) MarkEventProcessed(_ context.Context, eventID uuid.UUID, _ string) (bool, error) {
	if r.processed == nil {
		r.processed = make(map[uuid.UUID]bool)
	}
	if r.processed[eventID] {
		return false, nil
	}
	r.processed[eventID] = true
	return true, nil
}
func (r *fakeRepository) IsEventProcessed(_ context.Context, eventID uuid.UUID) (bool, error) {
	return r.processed[eventID], nil
}
func (*fakeRepository) Revenue(context.Context, reports.DateRange, string, string) (reports.RevenueSummary, error) {
	return reports.RevenueSummary{}, nil
}
func (*fakeRepository) TopProducts(context.Context, reports.DateRange, string, int) ([]reports.TopProductSales, error) {
	return nil, nil
}
func (*fakeRepository) Funnel(context.Context, reports.DateRange) ([]reports.DailyFunnel, error) {
	return nil, nil
}
func (*fakeRepository) LastProcessedAt(context.Context) (*time.Time, error) { return nil, nil }

type fakeTransactions struct{}

func (fakeTransactions) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type fakeSnapshots struct {
	snapshot reports.OrderAnalyticsSnapshot
}

func (f fakeSnapshots) GetAnalyticsSnapshot(_ context.Context, _ uuid.UUID) (reports.OrderAnalyticsSnapshot, error) {
	return f.snapshot, nil
}
func (f fakeSnapshots) GetSnapshotsByDateRange(context.Context, time.Time, time.Time) ([]reports.OrderAnalyticsSnapshot, error) {
	return []reports.OrderAnalyticsSnapshot{f.snapshot}, nil
}
func (fakeSnapshots) GetFunnelEventsByDateRange(context.Context, time.Time, time.Time) ([]reports.FunnelEventSnapshot, error) {
	return nil, nil
}
