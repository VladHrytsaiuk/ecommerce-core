package domain

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/pagination"
)

var (
	ErrPromoCodeNotFound = errors.New("promo code not found")
	ErrPromoCodeExpired  = errors.New("promo code has expired")
	ErrPromoCodeInactive = errors.New("promo code is inactive")
	ErrPromoCodeLimitExceeded = errors.New("promo code usage limit exceeded")
	ErrPromoCodeUserLimitExceeded = errors.New("user has exceeded usage limit for this promo code")
	ErrPromoCodeMinSubtotal = errors.New("minimum order subtotal for promo code not met")
	ErrPromoCodeInvalidCart = errors.New("cart does not contain any eligible items for this promo code")
)

type DiscountType string

const (
	DiscountTypePercent DiscountType = "percentage"
	DiscountTypeFixed   DiscountType = "fixed"
)

type PromoCode struct {
	ID               uuid.UUID  `json:"id" gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	Code             string     `json:"code" gorm:"uniqueIndex"`
	Description      string     `json:"description" gorm:"type:text"`
	DiscountType     DiscountType `json:"discount_type"`
	DiscountValue    int        `json:"discount_value"`
	IsActive         bool       `json:"is_active" gorm:"default:true"`
	StartsAt         *time.Time `json:"starts_at"`
	EndsAt           *time.Time `json:"ends_at"`
	UsageLimit       *int       `json:"usage_limit"`
	UsageCount       int        `json:"usage_count" gorm:"default:0"`
	UsageLimitPerUser *int       `json:"usage_limit_per_user" gorm:"default:1"`
	MinOrderSubtotal int        `json:"min_order_subtotal"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	DeletedAt        *time.Time `json:"deleted_at"`

	CategoryIDs []uuid.UUID `json:"category_ids,omitempty" gorm:"-"`
	BrandIDs    []uuid.UUID `json:"brand_ids,omitempty" gorm:"-"`
	ProductIDs  []uuid.UUID `json:"product_ids,omitempty" gorm:"-"`
}

func (PromoCode) TableName() string {
	return "promo_code"
}

type PromoCodeUsage struct {
	ID          uuid.UUID  `json:"id" gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	PromoCodeID uuid.UUID  `json:"promo_code_id"`
	OrderID     uuid.UUID  `json:"order_id"`
	UserID      *uuid.UUID `json:"user_id"`
	Email       *string    `json:"email"`
	Phone       *string    `json:"phone"`
	CreatedAt   time.Time  `json:"created_at"`
}

func (PromoCodeUsage) TableName() string {
	return "promo_code_usage"
}

// Interfaces

type PromoRepository interface {
	Create(ctx context.Context, p *PromoCode) error
	GetByID(ctx context.Context, id uuid.UUID) (*PromoCode, error)
	GetByCode(ctx context.Context, code string) (*PromoCode, error)
	Update(ctx context.Context, p *PromoCode) error
	Delete(ctx context.Context, id uuid.UUID) error
	List(ctx context.Context, page, limit int) ([]*PromoCode, int64, error)
	
	RecordUsage(ctx context.Context, usage *PromoCodeUsage) error
	CheckUserUsage(ctx context.Context, promoID uuid.UUID, userID *uuid.UUID, email, phone *string) (int, error)
	IncrementUsageCount(ctx context.Context, promoID uuid.UUID) error

	// For checkout atomic use
	IncrementUsageAtomic(ctx context.Context, promoID uuid.UUID) error
}

type PromoItemInfo struct {
	VariationID uuid.UUID
	ProductID   uuid.UUID
	CategoryID  uuid.UUID
	BrandID     *uuid.UUID
	Price       int
	Quantity    int
}

type PromoCalculationResult struct {
	TotalDiscountAmount int
	ItemDiscounts       map[uuid.UUID]int // VariationID -> Total Discount for this item
}

type PromoService interface {
	CreatePromo(ctx context.Context, p *PromoCode) (*PromoCode, error)
	GetPromoByID(ctx context.Context, id uuid.UUID) (*PromoCode, error)
	GetPromoByCode(ctx context.Context, code string) (*PromoCode, error)
	UpdatePromo(ctx context.Context, id uuid.UUID, p *PromoCode) (*PromoCode, error)
	DeletePromo(ctx context.Context, id uuid.UUID) error
	ListPromos(ctx context.Context, pgn pagination.Params) ([]PromoCode, pagination.Metadata, error)
	
	CalculateCartDiscount(ctx context.Context, promoID uuid.UUID, items []PromoItemInfo) (*PromoCalculationResult, error)
	ValidatePromoLimits(ctx context.Context, promoID uuid.UUID, userID *uuid.UUID, email, phone *string) error
}
