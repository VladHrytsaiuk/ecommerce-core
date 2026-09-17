// Package domain contains the promotion module's provider-neutral contracts.
package domain

import (
	"context"
	"errors"
	"time"

	ordersDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/orders/domain"
	"github.com/google/uuid"
)

var (
	ErrCodeNotFound   = errors.New("promo code not found")
	ErrCodeInactive   = errors.New("promo code is inactive")
	ErrCodeExpired    = errors.New("promo code has expired")
	ErrUsageExhausted = errors.New("promo code usage limit exceeded")
	ErrInvalidCode    = errors.New("invalid promo code")
)

const (
	TypePercent = "percent"
	TypeFixed   = "fixed"
)

type Code struct {
	ID            uuid.UUID
	Code          string
	DiscountType  string
	DiscountValue int64
	Currency      string
	IsActive      bool
	ValidUntil    *time.Time
	UsageLimit    *int
	UsageCount    int
}

// Repository represents both pricing reads and transaction-scoped redemption
// lifecycle. Reserve, Commit and Release require a workflow transaction in the
// context; this prevents any promotion state escaping the order transaction.
type Repository interface {
	FindByCode(context.Context, string) (*Code, error)
	Reserve(context.Context, uuid.UUID, ordersDomain.Promotion) error
	Commit(context.Context, uuid.UUID) error
	Release(context.Context, uuid.UUID) error
}

// AdminRepository is intentionally separate from the checkout/redemption
// port. Admin facades depend only on this narrow mutation capability.
type AdminRepository interface {
	Create(context.Context, Code) (*Code, error)
}

type AdminService interface {
	Create(context.Context, Code) (*Code, error)
}
