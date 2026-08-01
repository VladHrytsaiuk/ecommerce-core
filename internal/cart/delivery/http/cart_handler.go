package http

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/cart/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/helpers"
	mymiddleware "github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
)

// CartHandler обробляє HTTP запити для кошика
type CartHandler struct {
	service domain.CartService
	l       logger.Logger
}

// NewCartHandler створює новий інстанс хендлера
func NewCartHandler(s domain.CartService, l logger.Logger) *CartHandler {
	return &CartHandler{service: s, l: l}
}

// GetCart godoc
// @Summary      Get cart
// @Description  Get the shopping cart with all items and calculated totals. Uses JWT token for authenticated users or guest_session cookie for anonymous users.
// @Tags         Cart
// @Accept       json
// @Produce      json
// @Param        lang path string true "Language code (uk, en)"
// @Security     bearerAuth
// @Success      200  {object} CartResponse
// @Failure      400  {object} ErrorResponse
// @Failure      500  {object} ErrorResponse
// @Router       /api/{lang}/cart [get]
func (h *CartHandler) GetCart(c *gin.Context) {
	lang := mymiddleware.GetLanguage(c)
	userID, sessionID := helpers.GetIdentifiers(c)

	cart, variations, shipping, promoResult, promoCodeStr, err := h.service.GetFullCart(c.Request.Context(), userID, sessionID, lang)
	if err != nil {
		h.l.Errorw("Failed to get cart", "error", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "Internal Server Error",
			Message: "Failed to retrieve cart",
		})
		return
	}

	c.JSON(http.StatusOK, mapToCartResponse(cart, variations, shipping, promoResult, promoCodeStr, lang))
}

// AddToCart godoc
// @Summary      Add item to cart
// @Description  Add a product variation to the cart. If the item already exists, its quantity will be incremented.
// @Tags         Cart
// @Accept       json
// @Produce      json
// @Param        lang path string true "Language code (uk, en)"
// @Param        body body AddToCartRequest true "Item to add"
// @Security     bearerAuth
// @Success      201  {object} map[string]string
// @Failure      400  {object} ErrorResponse
// @Failure      404  {object} ErrorResponse
// @Failure      500  {object} ErrorResponse
// @Router       /api/{lang}/cart/items [post]
func (h *CartHandler) AddToCart(c *gin.Context) {
	var req AddToCartRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Bad Request",
			Message: "Invalid request body: variation_id and quantity (min 1) are required",
		})
		return
	}

	userID, sessionID := helpers.GetIdentifiers(c)
	if userID == nil && sessionID == nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Bad Request",
			Message: "Authorization token or session cookie is required",
		})
		return
	}

	err := h.service.AddItem(c.Request.Context(), userID, sessionID, req.VariationID, req.Quantity)
	if err != nil {
		if errors.Is(err, domain.ErrVariationNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Error:   "Not Found",
				Message: "Product variation not found",
			})
			return
		}
		if errors.Is(err, domain.ErrInvalidQuantity) {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error:   "Bad Request",
				Message: "Quantity must be greater than 0",
			})
			return
		}
		h.l.Errorw("Failed to add item to cart", "error", err, "variation_id", req.VariationID)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "Internal Server Error",
			Message: "Failed to add item to cart",
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Item added to cart"})
}

// UpdateCartItem godoc
// @Summary      Update cart item quantity
// @Description  Set the quantity of a specific product variation in the cart.
// @Tags         Cart
// @Accept       json
// @Produce      json
// @Param        lang path string true "Language code (uk, en)"
// @Param        variationId path string true "Product Variation UUID"
// @Param        body body UpdateCartItemRequest true "New quantity"
// @Security     bearerAuth
// @Success      200  {object} map[string]string
// @Failure      400  {object} ErrorResponse
// @Failure      404  {object} ErrorResponse
// @Failure      500  {object} ErrorResponse
// @Router       /api/{lang}/cart/items/{variationId} [patch]
func (h *CartHandler) UpdateCartItem(c *gin.Context) {
	variationIDStr := c.Param("variationId")
	variationID, err := uuid.Parse(variationIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Bad Request",
			Message: "Invalid variation UUID format",
		})
		return
	}

	var req UpdateCartItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Bad Request",
			Message: "Invalid request body: quantity (min 1) is required",
		})
		return
	}

	userID, sessionID := helpers.GetIdentifiers(c)
	if userID == nil && sessionID == nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Bad Request",
			Message: "Authorization token or session cookie is required",
		})
		return
	}

	err = h.service.UpdateQuantity(c.Request.Context(), userID, sessionID, variationID, req.Quantity)
	if err != nil {
		if errors.Is(err, domain.ErrCartItemNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Error:   "Not Found",
				Message: "Item not found in cart",
			})
			return
		}
		if errors.Is(err, domain.ErrInvalidQuantity) {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error:   "Bad Request",
				Message: "Quantity must be greater than 0",
			})
			return
		}
		h.l.Errorw("Failed to update cart item", "error", err, "variation_id", variationID)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "Internal Server Error",
			Message: "Failed to update cart item",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Cart item updated"})
}

// RemoveFromCart godoc
// @Summary      Remove item from cart
// @Description  Remove a product variation from the cart.
// @Tags         Cart
// @Accept       json
// @Produce      json
// @Param        lang path string true "Language code (uk, en)"
// @Param        variationId path string true "Product Variation UUID"
// @Security     bearerAuth
// @Success      200  {object} map[string]string
// @Failure      400  {object} ErrorResponse
// @Failure      404  {object} ErrorResponse
// @Failure      500  {object} ErrorResponse
// @Router       /api/{lang}/cart/items/{variationId} [delete]
func (h *CartHandler) RemoveFromCart(c *gin.Context) {
	variationIDStr := c.Param("variationId")
	variationID, err := uuid.Parse(variationIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Bad Request",
			Message: "Invalid variation UUID format",
		})
		return
	}

	userID, sessionID := helpers.GetIdentifiers(c)
	if userID == nil && sessionID == nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Bad Request",
			Message: "Authorization token or session cookie is required",
		})
		return
	}

	err = h.service.RemoveItem(c.Request.Context(), userID, sessionID, variationID)
	if err != nil {
		if errors.Is(err, domain.ErrCartItemNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Error:   "Not Found",
				Message: "Item not found in cart",
			})
			return
		}
		h.l.Errorw("Failed to remove item from cart", "error", err, "variation_id", variationID)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "Internal Server Error",
			Message: "Failed to remove item from cart",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Item removed from cart"})
}

// SyncCart godoc
// @Summary      Sync anonymous cart to user
// @Description  Moves all items from anonymous session cart to the authenticated user's cart. Quantities are merged if items overlap.
// @Tags         Cart
// @Accept       json
// @Produce      json
// @Param        lang path string true "Language code (uk, en)"
// @Param        body body SyncCartRequest false "Optional session_id to sync from (falls back to cookie)"
// @Security     bearerAuth
// @Success      200  {object} map[string]string
// @Failure      400  {object} ErrorResponse
// @Failure      401  {object} ErrorResponse
// @Failure      500  {object} ErrorResponse
// @Router       /api/{lang}/cart/sync [post]
func (h *CartHandler) SyncCart(c *gin.Context) {
	// Синхронізація доступна тільки для авторизованих
	userIDRaw, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "Unauthorized",
			Message: "Authentication required for sync",
		})
		return
	}

	var userID uuid.UUID
	switch v := userIDRaw.(type) {
	case string:
		userID, _ = uuid.Parse(v)
	case uuid.UUID:
		userID = v
	}

	if userID == uuid.Nil {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "Unauthorized",
			Message: "Invalid user token data",
		})
		return
	}

	var req SyncCartRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Якщо тіла немає — не страшно, спробуємо знайти в куках
		req.SessionID = ""
	}

	sessionID := req.SessionID
	if sessionID == "" {
		// Спробуємо взяти з куки/контексту
		sIDPtr := helpers.GetGuestSessionID(c)
		if sIDPtr != nil {
			sessionID = *sIDPtr
		}
	}

	if sessionID == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Bad Request",
			Message: "session_id is required in request body or as a cookie",
		})
		return
	}

	err := h.service.SyncSession(c.Request.Context(), sessionID, userID)
	if err != nil {
		h.l.Errorw("Failed to sync cart", "error", err, "session_id", sessionID, "user_id", userID)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "Internal Server Error",
			Message: "Failed to sync cart",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Cart synced successfully"})
}

// ApplyPromoCode godoc
// @Summary      Apply promo code
// @Description  Apply a promotional code to the cart
// @Tags         Cart
// @Accept       json
// @Produce      json
// @Param        lang path string true "Language code (uk, en)"
// @Param        body body ApplyPromoRequest true "Promo code"
// @Security     bearerAuth
// @Success      200  {object} CartResponse
// @Failure      400  {object} ErrorResponse
// @Failure      500  {object} ErrorResponse
// @Router       /api/{lang}/cart/promo [post]
func (h *CartHandler) ApplyPromoCode(c *gin.Context) {
	var req ApplyPromoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Bad Request",
			Message: "code is required",
		})
		return
	}

	lang := mymiddleware.GetLanguage(c)
	userID, sessionID := helpers.GetIdentifiers(c)

	if userID == nil && sessionID == nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Bad Request",
			Message: "Authorization token or session cookie is required",
		})
		return
	}

	err := h.service.ApplyPromoCode(c.Request.Context(), userID, sessionID, req.Code, lang)
	if err != nil {
		// Handle specific promo errors
		h.l.Errorw("Failed to apply promo code", "error", err, "code", req.Code)
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Bad Request",
			Message: "Validation failed",
			Details: []ValidationError{
				{Field: "promo_code", Message: err.Error()},
			},
		})
		return
	}

	// Fetch updated cart
	cart, variations, shipping, promoResult, promoCodeStr, err := h.service.GetFullCart(c.Request.Context(), userID, sessionID, lang)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "Internal Server Error",
			Message: "Failed to retrieve updated cart",
		})
		return
	}

	c.JSON(http.StatusOK, mapToCartResponse(cart, variations, shipping, promoResult, promoCodeStr, lang))
}

// RemovePromoCode godoc
// @Summary      Remove promo code
// @Description  Remove applied promotional code from the cart
// @Tags         Cart
// @Produce      json
// @Param        lang path string true "Language code (uk, en)"
// @Security     bearerAuth
// @Success      200  {object} map[string]string
// @Failure      400  {object} ErrorResponse
// @Failure      500  {object} ErrorResponse
// @Router       /api/{lang}/cart/promo [delete]
func (h *CartHandler) RemovePromoCode(c *gin.Context) {
	userID, sessionID := helpers.GetIdentifiers(c)

	if userID == nil && sessionID == nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Bad Request",
			Message: "Authorization token or session cookie is required",
		})
		return
	}

	err := h.service.RemovePromoCode(c.Request.Context(), userID, sessionID)
	if err != nil {
		h.l.Errorw("Failed to remove promo code", "error", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "Internal Server Error",
			Message: "Failed to remove promo code",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Promo code removed"})
}
