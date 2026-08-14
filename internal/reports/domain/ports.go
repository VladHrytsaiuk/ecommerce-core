package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// ReportsRepository owns reports-module persistence. MarkEventProcessed must
// use an atomic INSERT ... ON CONFLICT DO NOTHING and return true only for the
// worker that inserted the idempotency record. Projection handlers call it and
// UpsertDailySales in one database transaction.
type ReportsRepository interface {
	UpsertDailySales(context.Context, DailySales) error
	UpsertDailyProductSales(context.Context, DailyProductSales) error
	UpsertDailyFunnel(context.Context, DailyFunnel) error
	DeleteAggregates(context.Context, DateRange, string) error
	AcquireProjectionLock(context.Context) error
	MarkEventProcessed(context.Context, uuid.UUID, string) (inserted bool, err error)
	IsEventProcessed(context.Context, uuid.UUID) (bool, error)
	Revenue(context.Context, DateRange, string, string) (RevenueSummary, error)
	TopProducts(context.Context, DateRange, string, int) ([]TopProductSales, error)
	Funnel(context.Context, DateRange) ([]DailyFunnel, error)
	LastProcessedAt(context.Context) (*time.Time, error)
}

// QueryService is the read-only application boundary consumed by the Admin
// HTTP adapter.
type QueryService interface {
	Revenue(context.Context, DateRange, string) (RevenueSummary, error)
	TopProducts(context.Context, DateRange, string, int) ([]TopProductSales, error)
	Funnel(context.Context, DateRange) ([]DailyFunnel, error)
}

// DateRange is inclusive and deliberately date-only so report queries cannot
// make aggregate scans depend on client-controlled timestamps.
type DateRange struct {
	From time.Time
	To   time.Time
}

// TransactionManager supplies a module-neutral transaction boundary. The
// projection marker and all aggregate updates must share this boundary.
type TransactionManager interface {
	WithinTransaction(context.Context, func(context.Context) error) error
}

// OrderAnalyticsSnapshotProvider is a deliberately narrow anti-corruption
// port. Its Bootstrap-wired implementation may read an immutable order
// snapshot, but Reports never sees an Orders aggregate or repository.
type OrderAnalyticsSnapshotProvider interface {
	GetAnalyticsSnapshot(context.Context, uuid.UUID) (OrderAnalyticsSnapshot, error)
	GetSnapshotsByDateRange(context.Context, time.Time, time.Time) ([]OrderAnalyticsSnapshot, error)
	GetFunnelEventsByDateRange(context.Context, time.Time, time.Time) ([]FunnelEventSnapshot, error)
}

// Rebuilder starts a bounded asynchronous projection rebuild and exposes its
// state without leaking HTTP or worker concerns into the domain.
type Rebuilder interface {
	StartAsync(DateRange) (started bool, err error)
	Health(context.Context) (Health, error)
}

// OrderAnalyticsSnapshot holds only the immutable facts required for report
// projections. Monetary values are minor units in Currency.
type OrderAnalyticsSnapshot struct {
	OrderID uuid.UUID
	CartID  *uuid.UUID
	// Event IDs allow a rebuild to seed the same idempotency fence used by
	// Outbox projectors, including events published while the rebuild runs.
	PaidEventID     *uuid.UUID
	RefundedEventID *uuid.UUID

	CreatedAt   time.Time
	PaidAt      *time.Time
	CancelledAt *time.Time
	RefundedAt  *time.Time

	Currency              string
	Channel               string
	SubtotalMinor         int64
	TaxMinor              int64
	ShippingMinor         int64
	DiscountMinor         int64
	TotalMinor            int64
	RefundedTotalMinor    int64
	PurchasedProductItems []PurchasedProductItem
}

// PurchasedProductItem is an order-time line-item snapshot, not a live
// Catalog entity. ProductID/VariantID may be nil for historical deleted data.
type PurchasedProductItem struct {
	ProductID  *uuid.UUID
	VariantID  *uuid.UUID
	Quantity   int
	UnitMinor  int64
	TotalMinor int64
	Currency   string
}
