package http

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	adminApp "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/application"
	promosDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/promos/domain"

	"context"
)

// PromosFacade is the narrow slice of the promos admin facade this package
// needs, so the handler does not depend on the whole type.
type PromosFacade interface {
	Create(ctx context.Context, command adminApp.CreatePromoCommand) (*promosDomain.Code, error)
}

type createPromoRequest struct {
	Code          string     `json:"code" binding:"required,max=64"`
	DiscountType  string     `json:"discount_type" binding:"required"`
	DiscountValue int64      `json:"discount_value" binding:"required,gt=0"`
	Currency      string     `json:"currency"`
	IsActive      *bool      `json:"is_active"`
	ValidUntil    *time.Time `json:"valid_until"`
	UsageLimit    *int       `json:"usage_limit"`
}

// idempotency reads the caller's key for an audited mutation, or mints one.
//
// A generated key still deduplicates the event within a single request; it
// cannot deduplicate a client's retry, which is why callers that care about
// that send their own.
func idempotency(c *gin.Context) uuid.UUID {
	if key, err := uuid.Parse(c.GetHeader("Idempotency-Key")); err == nil && key != uuid.Nil {
		return key
	}
	return uuid.New()
}
