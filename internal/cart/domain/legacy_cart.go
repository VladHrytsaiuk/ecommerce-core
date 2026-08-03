//go:build legacy

package domain

import (
	"context"
	"errors"
	"time"

	discountDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/discount/domain"
	productDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/product/domain"
	"github.com/google/uuid"
)

var (
	ErrCartNotFound      = errors.New("cart not found")
	ErrCartItemNotFound  = errors.New("cart item not found")
	ErrVariationNotFound = errors.New("product variation not found")
	ErrNoIdentifier      = errors.New("either user_id or session_id must be provided")
	ErrInvalidQuantity   = errors.New("quantity must be greater than 0")
)

type Cart struct {
	ID          uuid.UUID  `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	UserID      *uuid.UUID `gorm:"type:uuid"`
	SessionID   *string    `gorm:"type:varchar(255)"`
	PromoCodeID *uuid.UUID `gorm:"type:uuid"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Items       []CartItem `gorm:"foreignKey:CartID"`
}

func (Cart) TableName() string { return "cart" }

type CartItem struct {
	ID          uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	CartID      uuid.UUID
	VariationID uuid.UUID
	Quantity    int
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Variation   productDomain.ProductVariation `gorm:"foreignKey:VariationID"`
}

func (CartItem) TableName() string { return "cart_item" }

type CartRepository interface {
	GetByUserID(context.Context, uuid.UUID, string) (*Cart, []productDomain.ProductVariation, error)
	GetBySessionID(context.Context, string, string) (*Cart, []productDomain.ProductVariation, error)
	AddItem(context.Context, *uuid.UUID, *string, uuid.UUID, int) error
	UpdateQuantity(context.Context, *uuid.UUID, *string, uuid.UUID, int) error
	RemoveItem(context.Context, *uuid.UUID, *string, uuid.UUID) error
	SyncSessionToUser(context.Context, string, uuid.UUID) error
	UpdatePromoCode(context.Context, *uuid.UUID, *string, *uuid.UUID) error
	DeleteExpiredAnonymous(context.Context, time.Time) error
	VariationExists(context.Context, uuid.UUID) (bool, error)
}
type CartService interface {
	GetFullCart(context.Context, *uuid.UUID, *string, string) (*Cart, []productDomain.ProductVariation, *ShippingSummary, *discountDomain.PromoCalculationResult, *string, error)
	AddItem(context.Context, *uuid.UUID, *string, uuid.UUID, int) error
	UpdateQuantity(context.Context, *uuid.UUID, *string, uuid.UUID, int) error
	RemoveItem(context.Context, *uuid.UUID, *string, uuid.UUID) error
	SyncSession(context.Context, string, uuid.UUID) error
	ApplyPromoCode(context.Context, *uuid.UUID, *string, string, string) error
	RemovePromoCode(context.Context, *uuid.UUID, *string) error
}
type CartSummary struct {
	TotalCount int
	TotalPrice int
}
type ShippingSummary struct {
	IsFreeShipping  bool
	Threshold       int
	RemainingAmount int
	MinOrderAmount  int
}
type CartItemDetail struct {
	VariationID uuid.UUID
	Quantity    int
	ItemPrice   int
	TotalPrice  int
}
