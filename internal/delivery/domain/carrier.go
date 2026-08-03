// Package domain defines provider-neutral delivery contracts.
package domain

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
)

// Carrier is implemented by a delivery adapter. It must not decide checkout,
// order-status, tax, or reservation rules.
type Carrier interface {
	Code() string
	Quote(context.Context, ShipmentQuoteRequest) ([]ShippingOption, error)
	CreateShipment(context.Context, CreateShipmentRequest) (ShipmentResult, error)
	Track(context.Context, TrackingRequest) (TrackingResult, error)
}

type Address struct {
	RecipientName  string
	RecipientPhone string
	CountryCode    string
	PostalCode     string
	City           string
	Line1          string
	Line2          string
	// LocalityID and ServicePointID are opaque identifiers supplied by the
	// chosen carrier's location lookup. They are not Nova Poshta-specific and
	// let every adapter validate its own service-point namespace.
	LocalityID     string
	ServicePointID string
}

type ShipmentItem struct {
	VariantID   uuid.UUID
	Quantity    int
	WeightGrams int
}

type ShipmentQuoteRequest struct {
	Destination Address
	Items       []ShipmentItem
	Currency    string
}

type ShippingOption struct {
	Code        string
	Title       string
	Amount      money.Money
	EstimatedAt *time.Time
}

type CreateShipmentRequest struct {
	OrderID        uuid.UUID
	IdempotencyKey string
	Destination    Address
	Items          []ShipmentItem
	DeclaredValue  money.Money
}

type ShipmentResult struct {
	ProviderReference string
	TrackingNumber    string
}

type TrackingRequest struct {
	TrackingNumber string
	RecipientPhone string
}

type TrackingResult struct {
	Status     string
	OccurredAt time.Time
}
