package domain

import (
	"context"
	"time"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
	"github.com/google/uuid"
)

const (
	StatusPendingPayment = "pending_payment"
	StatusPaid           = "paid"
	StatusCancelled      = "cancelled"
)

type Item struct {
	VariantID   *uuid.UUID
	ProductName string
	SKU         string
	Quantity    int
	UnitPrice   money.Money
	Total       money.Money
}
type Draft struct {
	Number           string
	CustomerID       *uuid.UUID
	Subtotal         money.Money
	Tax              money.Money
	Total            money.Money
	PaymentProvider  string
	DeliveryProvider string
	Items            []Item
}
type Order struct {
	ID               uuid.UUID
	Number           string
	CustomerID       *uuid.UUID
	Status           string
	Subtotal         money.Money
	Tax              money.Money
	Total            money.Money
	PaymentProvider  string
	DeliveryProvider string
	Items            []Item
	CreatedAt        time.Time
}
type Repository interface {
	Create(context.Context, *Order) error
}
type Service interface {
	Create(context.Context, Draft) (*Order, error)
}
