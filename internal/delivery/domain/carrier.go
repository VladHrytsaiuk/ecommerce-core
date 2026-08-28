// Package domain defines provider-neutral delivery contracts.
package domain

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
)

var (
	ErrLocationProviderUnavailable = errors.New("delivery location provider is not enabled")
	ErrInvalidLocationQuery        = errors.New("invalid delivery location query")
	ErrProviderUnavailable         = errors.New("delivery provider is temporarily unavailable")
)

// Carrier is implemented by a delivery adapter. It must not decide checkout,
// order-status, tax, or reservation rules.
type Carrier interface {
	Code() string
	Quote(context.Context, ShipmentQuoteRequest) ([]ShippingOption, error)
	CreateShipment(context.Context, CreateShipmentRequest) (ShipmentResult, error)
	Track(context.Context, TrackingRequest) (TrackingResult, error)
}

// ShipmentFinder is an optional capability of a carrier that can reconcile an
// ambiguous CreateShipment result using the stable idempotency reference sent
// to the provider. It deliberately remains separate from Carrier so existing
// providers do not pretend to support lookup semantics they do not have.
type ShipmentFinder interface {
	FindShipment(context.Context, string) (*ShipmentResult, error)
}

// LocationProvider is deliberately separate from Carrier: delivery providers
// that cannot expose selectable service points remain valid carriers.
type LocationProvider interface {
	ListAreas(context.Context) ([]Area, error)
	ListCities(context.Context, string) ([]City, error)
	ListServicePoints(context.Context, ServicePointQuery) (ServicePointPage, error)
}

type Area struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type City struct {
	ID     string `json:"id"`
	AreaID string `json:"area_id"`
	Name   string `json:"name"`
}

type ServicePoint struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Address string `json:"address"`
	Number  string `json:"number"`
	Kind    string `json:"kind"`
}

type ServicePointQuery struct {
	CityID string
	Kind   string
	Page   int
	Limit  int
}

type ServicePointPage struct {
	Items []ServicePoint `json:"items"`
	Page  int            `json:"page"`
	Limit int            `json:"limit"`
	Total int64          `json:"total"`
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
	// Status is the local delivery status. OrderStatusCode is the (optional)
	// operational order status to transition to after this result is persisted.
	Status          string
	OrderStatusCode string
	OccurredAt      time.Time
}
