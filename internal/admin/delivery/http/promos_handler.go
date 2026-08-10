package http

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	adminApp "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/application"
	adminDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/domain"
	sharedMiddleware "github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware"
	promosDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/promos/domain"
)

type PromosFacade interface {
	Create(c context.Context, command adminApp.CreatePromoCommand) (*promosDomain.Code, error)
}

// RegisterPromosRoutes only accepts requests after dynamic permission checks.
func RegisterPromosRoutes(group *gin.RouterGroup, authorizer adminDomain.Authorizer, facade PromosFacade) {
	if group == nil || facade == nil {
		return
	}
	group.POST("/promos", RequirePermission(authorizer, adminApp.PermissionPromosWrite), createPromo(facade))
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

func createPromo(facade PromosFacade) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := sharedMiddleware.AuthenticatedUserID(c)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "AUTHENTICATION_REQUIRED"})
			return
		}
		var request createPromoRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "INVALID_REQUEST"})
			return
		}
		eventKey := uuid.New()
		if header := strings.TrimSpace(c.GetHeader("Idempotency-Key")); header != "" {
			parsed, err := uuid.Parse(header)
			if err != nil {
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "INVALID_IDEMPOTENCY_KEY"})
				return
			}
			eventKey = parsed
		}
		active := true
		if request.IsActive != nil {
			active = *request.IsActive
		}
		promo, err := facade.Create(c.Request.Context(), adminApp.CreatePromoCommand{ActorUserID: userID, EventKey: eventKey, IPAddress: c.ClientIP(), Code: promosDomain.Code{Code: request.Code, DiscountType: request.DiscountType, DiscountValue: request.DiscountValue, Currency: request.Currency, IsActive: active, ValidUntil: request.ValidUntil, UsageLimit: request.UsageLimit}})
		if err != nil {
			switch err {
			case adminDomain.ErrNotAdmin, adminDomain.ErrPermissionDenied:
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "PERMISSION_DENIED"})
			case promosDomain.ErrInvalidCode:
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "INVALID_PROMO"})
			default:
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "CREATE_PROMO_FAILED"})
			}
			return
		}
		c.JSON(http.StatusCreated, gin.H{"id": promo.ID, "code": promo.Code})
	}
}
