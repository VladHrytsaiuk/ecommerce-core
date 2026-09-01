package domain

import (
	"errors"
	"fmt"
	"time"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
	"github.com/google/uuid"
)

var (
	ErrReturnOrderMismatch       = errors.New("return request order does not match order snapshot")
	ErrReturnCustomerMismatch    = errors.New("return request customer does not match order snapshot")
	ErrReturnOrderNotDelivered   = errors.New("order is not eligible for return in its current status")
	ErrReturnEligibilityDateMiss = errors.New("order has no delivery or payment date")
	ErrReturnWindowExpired       = errors.New("return eligibility window has expired")
)

// OrderSnapshot is the intentionally narrow contract supplied by Orders via
// Bootstrap. It contains no payment credentials, address data, or order model.
type OrderSnapshot struct {
	OrderID     uuid.UUID
	CustomerID  *uuid.UUID
	Status      string
	PaidAt      *time.Time
	DeliveredAt *time.Time
	Total       money.Money
	Items       []OrderItemSnapshot
}

type ReturnEligibilityPolicy interface {
	IsEligible(order OrderSnapshot, request ReturnRequest) error
}

// WindowEligibilityPolicy applies a calendar-day return window. Delivery time
// takes precedence; payment time provides a safe fallback for historic orders
// whose carrier timestamp was unavailable.
type WindowEligibilityPolicy struct {
	windowDays int
	now        func() time.Time
}

func NewWindowEligibilityPolicy(windowDays int) (*WindowEligibilityPolicy, error) {
	if windowDays <= 0 || windowDays > 3650 {
		return nil, fmt.Errorf("return window days must be between 1 and 3650")
	}
	return &WindowEligibilityPolicy{windowDays: windowDays, now: time.Now}, nil
}

func (p *WindowEligibilityPolicy) IsEligible(order OrderSnapshot, request ReturnRequest) error {
	if p == nil || p.now == nil || p.windowDays <= 0 {
		return errors.New("return eligibility policy is not configured")
	}
	if order.OrderID == uuid.Nil || request.OrderID != order.OrderID {
		return ErrReturnOrderMismatch
	}
	if order.CustomerID == nil || *order.CustomerID == uuid.Nil || request.CustomerID != *order.CustomerID {
		return ErrReturnCustomerMismatch
	}
	if order.Status != "delivered" {
		return ErrReturnOrderNotDelivered
	}

	basis := order.PaidAt
	if order.DeliveredAt != nil {
		basis = order.DeliveredAt
	}
	if basis == nil || basis.IsZero() {
		return ErrReturnEligibilityDateMiss
	}
	if p.now().UTC().After(basis.UTC().AddDate(0, 0, p.windowDays)) {
		return ErrReturnWindowExpired
	}
	return nil
}
