// Package application contains Reports query use cases. It keeps HTTP input
// validation and repository aggregation separate from the transport adapter.
package application

import (
	"context"
	"fmt"
	"strings"
	"time"

	reports "github.com/VladHrytsaiuk/ecommerce-core/internal/reports/domain"
)

const (
	maxQueryDays       = 366
	defaultTopProducts = 10
	maxTopProducts     = 100
)

type QueryService struct {
	repository reports.ReportsRepository
	timezone   string
}

func NewQueryService(repository reports.ReportsRepository, timezone string) (*QueryService, error) {
	if repository == nil {
		return nil, fmt.Errorf("reports repository is required")
	}
	timezone = strings.TrimSpace(timezone)
	if timezone == "" {
		timezone = "UTC"
	}
	if _, err := time.LoadLocation(timezone); err != nil {
		return nil, fmt.Errorf("reports timezone is invalid: %w", err)
	}
	return &QueryService{repository: repository, timezone: timezone}, nil
}

func (s *QueryService) Revenue(ctx context.Context, period reports.DateRange, currency string) (reports.RevenueSummary, error) {
	if err := validateQuery(period, currency); err != nil {
		return reports.RevenueSummary{}, err
	}
	return s.repository.Revenue(ctx, normalizeRange(period), strings.ToUpper(strings.TrimSpace(currency)), s.timezone)
}

func (s *QueryService) TopProducts(ctx context.Context, period reports.DateRange, currency string, limit int) ([]reports.TopProductSales, error) {
	if err := validateQuery(period, currency); err != nil {
		return nil, err
	}
	if limit == 0 {
		limit = defaultTopProducts
	}
	if limit < 1 || limit > maxTopProducts {
		return nil, fmt.Errorf("reports top-products limit must be between 1 and %d", maxTopProducts)
	}
	return s.repository.TopProducts(ctx, normalizeRange(period), strings.ToUpper(strings.TrimSpace(currency)), limit)
}

func (s *QueryService) Funnel(ctx context.Context, period reports.DateRange) ([]reports.DailyFunnel, error) {
	if period.From.IsZero() || period.To.IsZero() || period.To.Before(period.From) || period.To.Sub(period.From) > maxQueryDays*24*time.Hour {
		return nil, fmt.Errorf("reports date range is invalid")
	}
	return s.repository.Funnel(ctx, normalizeRange(period))
}

func validateQuery(period reports.DateRange, currency string) error {
	if period.From.IsZero() || period.To.IsZero() || period.To.Before(period.From) {
		return fmt.Errorf("reports date range is invalid")
	}
	if period.To.Sub(period.From) > maxQueryDays*24*time.Hour {
		return fmt.Errorf("reports date range must not exceed %d days", maxQueryDays)
	}
	currency = strings.TrimSpace(currency)
	if len(currency) != 3 {
		return fmt.Errorf("reports currency must be a three-letter ISO 4217 code")
	}
	for _, char := range currency {
		if (char < 'A' || char > 'Z') && (char < 'a' || char > 'z') {
			return fmt.Errorf("reports currency must be a three-letter ISO 4217 code")
		}
	}
	return nil
}

func normalizeRange(period reports.DateRange) reports.DateRange {
	return reports.DateRange{
		From: time.Date(period.From.Year(), period.From.Month(), period.From.Day(), 0, 0, 0, 0, time.UTC),
		To:   time.Date(period.To.Year(), period.To.Month(), period.To.Day(), 0, 0, 0, 0, time.UTC),
	}
}

var _ interface {
	Revenue(context.Context, reports.DateRange, string) (reports.RevenueSummary, error)
	TopProducts(context.Context, reports.DateRange, string, int) ([]reports.TopProductSales, error)
	Funnel(context.Context, reports.DateRange) ([]reports.DailyFunnel, error)
} = (*QueryService)(nil)
