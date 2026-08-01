package http

import (
	"time"

	"github.com/google/uuid"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/discount/domain"
)

// CreatePromoRequest DTO для створення промокоду
type CreatePromoRequest struct {
	Code              string      `json:"code" binding:"required"`
	Description       string      `json:"description"`
	DiscountType      string      `json:"discount_type" binding:"required,oneof=percentage fixed"`
	DiscountValue     int         `json:"discount_value" binding:"required,min=1"`
	MinOrderSubtotal  int         `json:"min_order_subtotal"`
	UsageLimit        *int        `json:"usage_limit"`
	UsageLimitPerUser *int        `json:"usage_limit_per_user"`
	StartsAt          *time.Time  `json:"starts_at"`
	EndsAt            *time.Time  `json:"ends_at"`
	IsActive          bool        `json:"is_active"`
	CategoryIDs       []uuid.UUID `json:"category_ids"`
	BrandIDs          []uuid.UUID `json:"brand_ids"`
	ProductIDs        []uuid.UUID `json:"product_ids"`
}

// UpdatePromoRequest DTO для оновлення промокоду
type UpdatePromoRequest struct {
	Description       *string      `json:"description"`
	DiscountType      *string      `json:"discount_type" binding:"omitempty,oneof=percentage fixed"`
	DiscountValue     *int         `json:"discount_value" binding:"omitempty,min=1"`
	MinOrderSubtotal  *int         `json:"min_order_subtotal"`
	UsageLimit        *int         `json:"usage_limit"`
	UsageLimitPerUser *int         `json:"usage_limit_per_user"`
	StartsAt          *time.Time   `json:"starts_at"`
	EndsAt            *time.Time   `json:"ends_at"`
	IsActive          *bool        `json:"is_active"`
	CategoryIDs       *[]uuid.UUID `json:"category_ids"`
	BrandIDs          *[]uuid.UUID `json:"brand_ids"`
	ProductIDs        *[]uuid.UUID `json:"product_ids"`
}

// PromoResponse DTO відповіді
type PromoResponse struct {
	ID                uuid.UUID   `json:"id"`
	Code              string      `json:"code"`
	Description       string      `json:"description"`
	DiscountType      string      `json:"discount_type"`
	DiscountValue     int         `json:"discount_value"`
	MinOrderSubtotal  int         `json:"min_order_subtotal"`
	UsageLimit        *int        `json:"usage_limit"`
	UsageCount        int         `json:"usage_count"`
	UsageLimitPerUser *int        `json:"usage_limit_per_user"`
	StartsAt          *time.Time  `json:"starts_at"`
	EndsAt            *time.Time  `json:"ends_at"`
	IsActive          bool        `json:"is_active"`
	CreatedAt         time.Time   `json:"created_at"`
	UpdatedAt         time.Time   `json:"updated_at"`

	CategoryIDs []uuid.UUID `json:"category_ids"`
	BrandIDs    []uuid.UUID `json:"brand_ids"`
	ProductIDs  []uuid.UUID `json:"product_ids"`
}

func mapToPromoResponse(p *domain.PromoCode) PromoResponse {
	resp := PromoResponse{
		ID:                p.ID,
		Code:              p.Code,
		Description:       p.Description,
		DiscountType:      string(p.DiscountType),
		DiscountValue:     p.DiscountValue,
		MinOrderSubtotal:  p.MinOrderSubtotal,
		UsageLimit:        p.UsageLimit,
		UsageCount:        p.UsageCount,
		UsageLimitPerUser: p.UsageLimitPerUser,
		StartsAt:          p.StartsAt,
		EndsAt:            p.EndsAt,
		IsActive:          p.IsActive,
		CreatedAt:         p.CreatedAt,
		UpdatedAt:         p.UpdatedAt,
	}

	if p.CategoryIDs != nil {
		resp.CategoryIDs = p.CategoryIDs
	} else {
		resp.CategoryIDs = []uuid.UUID{}
	}

	if p.BrandIDs != nil {
		resp.BrandIDs = p.BrandIDs
	} else {
		resp.BrandIDs = []uuid.UUID{}
	}

	if p.ProductIDs != nil {
		resp.ProductIDs = p.ProductIDs
	} else {
		resp.ProductIDs = []uuid.UUID{}
	}

	return resp
}
