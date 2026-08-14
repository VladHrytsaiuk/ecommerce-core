// Package domain contains the provider-neutral CQRS contracts owned by the
// Reports module. It never imports Orders, Catalog, or their repositories.
package domain

import (
	"time"

	"github.com/google/uuid"
)

// ProcessedEvent is the durable idempotency record for an Outbox delivery.
// EventID is globally stable and must be inserted before a projection mutates
// an aggregate in the same SQL transaction.
type ProcessedEvent struct {
	EventID     uuid.UUID
	Topic       string
	ProcessedAt time.Time
}

// DailySales is one currency-specific, timezone-specific sales bucket. All
// money values are ISO-4217 minor units; currencies must never be summed.
type DailySales struct {
	BucketDate time.Time
	Timezone   string
	Currency   string
	Channel    string

	PaidOrdersCount      int64
	CancelledOrdersCount int64
	RefundedOrdersCount  int64
	GrossRevenueMinor    int64
	RefundRevenueMinor   int64
	NetRevenueMinor      int64
	UpdatedAt            time.Time
}

// DailyProductSales is the product-level counterpart to DailySales. A nil
// VariantID represents an order line whose variant was later deleted; it is
// still a valid immutable product-sales bucket.
type DailyProductSales struct {
	BucketDate        time.Time
	Currency          string
	ProductID         uuid.UUID
	VariantID         *uuid.UUID
	UnitsSold         int64
	GrossRevenueMinor int64
	NetRevenueMinor   int64
	UpdatedAt         time.Time
}

// DailyFunnel counts each immutable lifecycle event exactly once per channel.
type DailyFunnel struct {
	BucketDate       time.Time
	Channel          string
	CartsCreated     int64
	CheckoutsStarted int64
	OrdersPaid       int64
}

// FunnelEventSnapshot is the PII-free lifecycle fact needed to rebuild the
// funnel projection from the durable Outbox event journal.
type FunnelEventSnapshot struct {
	EventID    uuid.UUID
	Topic      string
	OccurredAt time.Time
	Channel    string
}

// Health describes the observable state of the optional Reports module.
type Health struct {
	RebuildActive        bool       `json:"rebuild_active"`
	LastEventProcessedAt *time.Time `json:"last_event_processed_at,omitempty"`
}

// RevenueSummary is the range aggregate returned to the protected Admin API.
type RevenueSummary struct {
	Currency             string
	PaidOrdersCount      int64
	CancelledOrdersCount int64
	RefundedOrdersCount  int64
	GrossRevenueMinor    int64
	RefundRevenueMinor   int64
	NetRevenueMinor      int64
}

// TopProductSales is one ranked product result for a reporting period.
type TopProductSales struct {
	ProductID         uuid.UUID
	UnitsSold         int64
	GrossRevenueMinor int64
	NetRevenueMinor   int64
}
