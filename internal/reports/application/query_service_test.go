package application

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	reports "github.com/VladHrytsaiuk/ecommerce-core/internal/reports/domain"
)

func TestQueryServiceRestrictsRangeAndUsesConfiguredTimezone(t *testing.T) {
	t.Parallel()
	repository := &queryRepositoryFake{}
	service, err := NewQueryService(repository, "Europe/Kyiv")
	if err != nil {
		t.Fatal(err)
	}
	period := reports.DateRange{From: time.Date(2026, 1, 1, 12, 0, 0, 0, time.FixedZone("ignored", 3600)), To: time.Date(2026, 1, 31, 12, 0, 0, 0, time.UTC)}
	if _, err := service.Revenue(context.Background(), period, "uah"); err != nil {
		t.Fatalf("Revenue() error = %v", err)
	}
	if repository.timezone != "Europe/Kyiv" || repository.currency != "UAH" || repository.period.From.Hour() != 0 {
		t.Fatalf("repository query = %+v / %q / %q", repository.period, repository.currency, repository.timezone)
	}
	tooLong := reports.DateRange{From: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)}
	if _, err := service.Revenue(context.Background(), tooLong, "UAH"); err == nil {
		t.Fatal("Revenue() error = nil, want range validation error")
	}
}

type queryRepositoryFake struct {
	period   reports.DateRange
	currency string
	timezone string
}

func (*queryRepositoryFake) UpsertDailySales(context.Context, reports.DailySales) error { return nil }
func (*queryRepositoryFake) UpsertDailyProductSales(context.Context, reports.DailyProductSales) error {
	return nil
}
func (*queryRepositoryFake) MarkEventProcessed(context.Context, uuid.UUID, string) (bool, error) {
	return false, nil
}
func (*queryRepositoryFake) IsEventProcessed(context.Context, uuid.UUID) (bool, error) {
	return false, nil
}
func (r *queryRepositoryFake) Revenue(_ context.Context, period reports.DateRange, currency, timezone string) (reports.RevenueSummary, error) {
	r.period, r.currency, r.timezone = period, currency, timezone
	return reports.RevenueSummary{Currency: currency}, nil
}
func (*queryRepositoryFake) TopProducts(context.Context, reports.DateRange, string, int) ([]reports.TopProductSales, error) {
	return nil, nil
}
func (*queryRepositoryFake) UpsertDailyFunnel(context.Context, reports.DailyFunnel) error { return nil }
func (*queryRepositoryFake) DeleteAggregates(context.Context, reports.DateRange, string) error {
	return nil
}
func (*queryRepositoryFake) AcquireProjectionLock(context.Context) error { return nil }
func (*queryRepositoryFake) Funnel(context.Context, reports.DateRange) ([]reports.DailyFunnel, error) {
	return nil, nil
}
func (*queryRepositoryFake) LastProcessedAt(context.Context) (*time.Time, error) { return nil, nil }

var _ reports.ReportsRepository = (*queryRepositoryFake)(nil)
