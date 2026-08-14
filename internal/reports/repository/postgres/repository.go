// Package postgres persists Reports-owned CQRS aggregates in PostgreSQL.
package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	transaction "github.com/VladHrytsaiuk/ecommerce-core/internal/platform/postgres/transaction"
	reports "github.com/VladHrytsaiuk/ecommerce-core/internal/reports/domain"
)

type Repository struct{ db *gorm.DB }

const projectionAdvisoryLock int64 = 4_218_006_014

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

// MarkEventProcessed atomically fences an at-least-once delivery. Callers must
// use it and the following aggregate mutation in one transaction.
func (r *Repository) MarkEventProcessed(ctx context.Context, eventID uuid.UUID, topic string) (bool, error) {
	if r == nil || r.db == nil || eventID == uuid.Nil || strings.TrimSpace(topic) == "" {
		return false, fmt.Errorf("invalid processed report event")
	}
	result := r.database(ctx).Exec(`
INSERT INTO report_processed_events (event_id, topic)
VALUES (?, ?)
ON CONFLICT (event_id) DO NOTHING`, eventID, strings.TrimSpace(topic))
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

func (r *Repository) IsEventProcessed(ctx context.Context, eventID uuid.UUID) (bool, error) {
	if r == nil || r.db == nil || eventID == uuid.Nil {
		return false, fmt.Errorf("invalid processed report event ID")
	}
	var exists bool
	if err := r.database(ctx).Raw(`SELECT EXISTS(SELECT 1 FROM report_processed_events WHERE event_id = ?)`, eventID).Scan(&exists).Error; err != nil {
		return false, err
	}
	return exists, nil
}

func (r *Repository) UpsertDailySales(ctx context.Context, sales reports.DailySales) error {
	if r == nil || r.db == nil || sales.BucketDate.IsZero() || strings.TrimSpace(sales.Timezone) == "" || strings.TrimSpace(sales.Currency) == "" || strings.TrimSpace(sales.Channel) == "" {
		return fmt.Errorf("invalid daily sales aggregate")
	}
	if sales.PaidOrdersCount < 0 || sales.CancelledOrdersCount < 0 || sales.RefundedOrdersCount < 0 || sales.GrossRevenueMinor < 0 || sales.RefundRevenueMinor < 0 || sales.NetRevenueMinor != sales.GrossRevenueMinor-sales.RefundRevenueMinor {
		return fmt.Errorf("invalid daily sales metrics")
	}
	return r.database(ctx).Exec(`
INSERT INTO report_daily_sales (
    bucket_date, timezone, currency, channel,
    paid_orders_count, cancelled_orders_count, refunded_orders_count,
    gross_revenue_minor, refund_revenue_minor, net_revenue_minor, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT (bucket_date, timezone, currency, channel) DO UPDATE SET
    paid_orders_count = report_daily_sales.paid_orders_count + EXCLUDED.paid_orders_count,
    cancelled_orders_count = report_daily_sales.cancelled_orders_count + EXCLUDED.cancelled_orders_count,
    refunded_orders_count = report_daily_sales.refunded_orders_count + EXCLUDED.refunded_orders_count,
    gross_revenue_minor = report_daily_sales.gross_revenue_minor + EXCLUDED.gross_revenue_minor,
    refund_revenue_minor = report_daily_sales.refund_revenue_minor + EXCLUDED.refund_revenue_minor,
    net_revenue_minor = report_daily_sales.net_revenue_minor + EXCLUDED.net_revenue_minor,
    updated_at = CURRENT_TIMESTAMP`,
		sales.BucketDate.Format("2006-01-02"), strings.TrimSpace(sales.Timezone), strings.ToUpper(strings.TrimSpace(sales.Currency)), strings.TrimSpace(sales.Channel),
		sales.PaidOrdersCount, sales.CancelledOrdersCount, sales.RefundedOrdersCount,
		sales.GrossRevenueMinor, sales.RefundRevenueMinor, sales.NetRevenueMinor,
	).Error
}

func (r *Repository) UpsertDailyProductSales(ctx context.Context, sales reports.DailyProductSales) error {
	if r == nil || r.db == nil || sales.BucketDate.IsZero() || sales.ProductID == uuid.Nil || strings.TrimSpace(sales.Currency) == "" {
		return fmt.Errorf("invalid daily product sales aggregate")
	}
	if sales.UnitsSold < 0 || sales.GrossRevenueMinor < 0 {
		return fmt.Errorf("invalid daily product sales metrics")
	}
	return r.database(ctx).Exec(`
INSERT INTO report_daily_product_sales (
    bucket_date, currency, product_id, variant_id,
    units_sold, gross_revenue_minor, net_revenue_minor, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT (bucket_date, currency, product_id, variant_id) DO UPDATE SET
    units_sold = report_daily_product_sales.units_sold + EXCLUDED.units_sold,
    gross_revenue_minor = report_daily_product_sales.gross_revenue_minor + EXCLUDED.gross_revenue_minor,
    net_revenue_minor = report_daily_product_sales.net_revenue_minor + EXCLUDED.net_revenue_minor,
    updated_at = CURRENT_TIMESTAMP`,
		sales.BucketDate.Format("2006-01-02"), strings.ToUpper(strings.TrimSpace(sales.Currency)), sales.ProductID, sales.VariantID,
		sales.UnitsSold, sales.GrossRevenueMinor, sales.NetRevenueMinor,
	).Error
}

func (r *Repository) UpsertDailyFunnel(ctx context.Context, funnel reports.DailyFunnel) error {
	if r == nil || r.db == nil || funnel.BucketDate.IsZero() || strings.TrimSpace(funnel.Channel) == "" || funnel.CartsCreated < 0 || funnel.CheckoutsStarted < 0 || funnel.OrdersPaid < 0 {
		return fmt.Errorf("invalid daily funnel aggregate")
	}
	return r.database(ctx).Exec(`
INSERT INTO report_daily_funnel (bucket_date, channel, carts_created, checkouts_started, orders_paid, updated_at)
VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT (bucket_date, channel) DO UPDATE SET
    carts_created = report_daily_funnel.carts_created + EXCLUDED.carts_created,
    checkouts_started = report_daily_funnel.checkouts_started + EXCLUDED.checkouts_started,
    orders_paid = report_daily_funnel.orders_paid + EXCLUDED.orders_paid,
    updated_at = CURRENT_TIMESTAMP`, funnel.BucketDate.Format("2006-01-02"), strings.TrimSpace(funnel.Channel), funnel.CartsCreated, funnel.CheckoutsStarted, funnel.OrdersPaid).Error
}

// DeleteAggregates removes exactly one reporting window before a rebuild. The
// caller must already hold AcquireProjectionLock in the same transaction.
func (r *Repository) DeleteAggregates(ctx context.Context, period reports.DateRange, timezone string) error {
	if r == nil || r.db == nil || !validRange(period) || strings.TrimSpace(timezone) == "" {
		return fmt.Errorf("invalid reports rebuild range")
	}
	db := r.database(ctx)
	from, to := period.From.Format("2006-01-02"), period.To.Format("2006-01-02")
	if err := db.Exec(`DELETE FROM report_daily_sales WHERE bucket_date >= ? AND bucket_date <= ? AND timezone = ?`, from, to, strings.TrimSpace(timezone)).Error; err != nil {
		return err
	}
	if err := db.Exec(`DELETE FROM report_daily_product_sales WHERE bucket_date >= ? AND bucket_date <= ?`, from, to).Error; err != nil {
		return err
	}
	return db.Exec(`DELETE FROM report_daily_funnel WHERE bucket_date >= ? AND bucket_date <= ?`, from, to).Error
}

// AcquireProjectionLock serializes rebuilds and ordinary Outbox projections
// across application replicas. PostgreSQL releases this transaction-scoped
// advisory lock automatically on commit or rollback.
func (r *Repository) AcquireProjectionLock(ctx context.Context) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("reports repository is not configured")
	}
	return r.database(ctx).Exec(`SELECT pg_advisory_xact_lock(?)`, projectionAdvisoryLock).Error
}

func (r *Repository) Revenue(ctx context.Context, period reports.DateRange, currency, timezone string) (reports.RevenueSummary, error) {
	if r == nil || r.db == nil || !validRange(period) || strings.TrimSpace(currency) == "" || strings.TrimSpace(timezone) == "" {
		return reports.RevenueSummary{}, fmt.Errorf("invalid reports revenue query")
	}
	var summary reports.RevenueSummary
	if err := r.database(ctx).Raw(`
SELECT ? AS currency,
       COALESCE(SUM(paid_orders_count), 0) AS paid_orders_count,
       COALESCE(SUM(cancelled_orders_count), 0) AS cancelled_orders_count,
       COALESCE(SUM(refunded_orders_count), 0) AS refunded_orders_count,
       COALESCE(SUM(gross_revenue_minor), 0) AS gross_revenue_minor,
       COALESCE(SUM(refund_revenue_minor), 0) AS refund_revenue_minor,
       COALESCE(SUM(net_revenue_minor), 0) AS net_revenue_minor
FROM report_daily_sales
WHERE bucket_date >= ? AND bucket_date <= ? AND currency = ? AND timezone = ?`,
		strings.ToUpper(strings.TrimSpace(currency)), period.From.Format("2006-01-02"), period.To.Format("2006-01-02"), strings.ToUpper(strings.TrimSpace(currency)), strings.TrimSpace(timezone)).Scan(&summary).Error; err != nil {
		return reports.RevenueSummary{}, err
	}
	return summary, nil
}

func (r *Repository) TopProducts(ctx context.Context, period reports.DateRange, currency string, limit int) ([]reports.TopProductSales, error) {
	if r == nil || r.db == nil || !validRange(period) || strings.TrimSpace(currency) == "" || limit < 1 || limit > 100 {
		return nil, fmt.Errorf("invalid reports top products query")
	}
	var results []reports.TopProductSales
	if err := r.database(ctx).Raw(`
SELECT product_id,
       SUM(units_sold) AS units_sold,
       SUM(gross_revenue_minor) AS gross_revenue_minor,
       SUM(net_revenue_minor) AS net_revenue_minor
FROM report_daily_product_sales
WHERE bucket_date >= ? AND bucket_date <= ? AND currency = ?
GROUP BY product_id
ORDER BY SUM(net_revenue_minor) DESC, product_id ASC
LIMIT ?`, period.From.Format("2006-01-02"), period.To.Format("2006-01-02"), strings.ToUpper(strings.TrimSpace(currency)), limit).Scan(&results).Error; err != nil {
		return nil, err
	}
	return results, nil
}

func (r *Repository) Funnel(ctx context.Context, period reports.DateRange) ([]reports.DailyFunnel, error) {
	if r == nil || r.db == nil || !validRange(period) {
		return nil, fmt.Errorf("invalid reports funnel query")
	}
	var rows []reports.DailyFunnel
	if err := r.database(ctx).Raw(`
SELECT bucket_date, channel,
       SUM(carts_created) AS carts_created,
       SUM(checkouts_started) AS checkouts_started,
       SUM(orders_paid) AS orders_paid
FROM report_daily_funnel
WHERE bucket_date >= ? AND bucket_date <= ?
GROUP BY bucket_date, channel
ORDER BY bucket_date ASC, channel ASC`, period.From.Format("2006-01-02"), period.To.Format("2006-01-02")).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *Repository) LastProcessedAt(ctx context.Context) (*time.Time, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("reports repository is not configured")
	}
	var result struct{ ProcessedAt *time.Time }
	if err := r.database(ctx).Raw(`SELECT MAX(processed_at) AS processed_at FROM report_processed_events`).Scan(&result).Error; err != nil {
		return nil, err
	}
	return result.ProcessedAt, nil
}

// WithinTransaction joins an existing transaction from context instead of
// creating a GORM savepoint. Otherwise it creates and propagates one.
func (r *Repository) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	if r == nil || r.db == nil || fn == nil {
		return fmt.Errorf("reports transaction is not configured")
	}
	if tx, err := transaction.FromContext(ctx); err == nil {
		return fn(transaction.WithContext(ctx, tx))
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(transaction.WithContext(ctx, tx))
	})
}

func (r *Repository) database(ctx context.Context) *gorm.DB {
	if tx, err := transaction.FromContext(ctx); err == nil {
		return tx.WithContext(ctx)
	}
	return r.db.WithContext(ctx)
}

func validRange(period reports.DateRange) bool {
	return !period.From.IsZero() && !period.To.IsZero() && !period.To.Before(period.From)
}

var _ reports.ReportsRepository = (*Repository)(nil)
var _ reports.TransactionManager = (*Repository)(nil)
