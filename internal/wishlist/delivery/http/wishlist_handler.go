package http

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/helpers"
	mymiddleware "github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/wishlist/domain"
)

// WishlistHandler обробляє HTTP запити для вішліста
type WishlistHandler struct {
	service domain.WishlistService
	l       logger.Logger
}

// NewWishlistHandler створює новий інстанс хендлера
func NewWishlistHandler(s domain.WishlistService, l logger.Logger) *WishlistHandler {
	return &WishlistHandler{service: s, l: l}
}

// GetWishlist godoc
// @Summary      Get wishlist items
// @Description  Get the list of product variations in the wishlist. Uses JWT token for authenticated users or wishlist_session cookie for anonymous users.
// @Tags         Wishlist
// @Accept       json
// @Produce      json
// @Param        lang path string true "Language code (uk, en)"
// @Security     bearerAuth
// @Success      200  {object} map[string]interface{} "data: []WishlistItemResponse"
// @Failure      400  {object} ErrorResponse
// @Failure      500  {object} ErrorResponse
// @Router       /api/{lang}/wishlist [get]
func (h *WishlistHandler) GetWishlist(c *gin.Context) {
	lang := mymiddleware.GetLanguage(c)
	userID, sessionID := helpers.GetIdentifiers(c)

	// Якщо немає ідентифікаторів — повертаємо порожній вішліст (аналогічно GetCart)
	if userID == nil && sessionID == nil {
		c.JSON(http.StatusOK, gin.H{
			"data": []WishlistItemResponse{},
		})
		return
	}

	variations, err := h.service.GetItems(c.Request.Context(), userID, sessionID, lang)
	if err != nil {
		h.l.Errorw("Failed to get wishlist items", "error", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "Internal Server Error",
			Message: "Failed to retrieve wishlist items",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": mapVariationsToWishlistResponse(variations, lang),
	})
}

// AddToWishlist godoc
// @Summary      Add item to wishlist
// @Description  Add a product variation to the wishlist. Idempotent — adding the same item twice is a no-op.
// @Tags         Wishlist
// @Accept       json
// @Produce      json
// @Param        lang path string true "Language code (uk, en)"
// @Param        variationId path string true "Product Variation UUID"
// @Security     bearerAuth
// @Success      201  {object} map[string]string
// @Failure      400  {object} ErrorResponse
// @Failure      404  {object} ErrorResponse
// @Failure      500  {object} ErrorResponse
// @Router       /api/{lang}/wishlist/{variationId} [post]
func (h *WishlistHandler) AddToWishlist(c *gin.Context) {
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

	err = h.service.AddItem(c.Request.Context(), userID, sessionID, variationID)
	if err != nil {
		if errors.Is(err, domain.ErrVariationNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Error:   "Not Found",
				Message: "Product variation not found",
			})
			return
		}
		h.l.Errorw("Failed to add item to wishlist", "error", err, "variation_id", variationID)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "Internal Server Error",
			Message: "Failed to add item to wishlist",
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Item added to wishlist"})
}

// RemoveFromWishlist godoc
// @Summary      Remove item from wishlist
// @Description  Remove a product variation from the wishlist.
// @Tags         Wishlist
// @Accept       json
// @Produce      json
// @Param        lang path string true "Language code (uk, en)"
// @Param        variationId path string true "Product Variation UUID"
// @Security     bearerAuth
// @Success      200  {object} map[string]string
// @Failure      400  {object} ErrorResponse
// @Failure      404  {object} ErrorResponse
// @Failure      500  {object} ErrorResponse
// @Router       /api/{lang}/wishlist/{variationId} [delete]
func (h *WishlistHandler) RemoveFromWishlist(c *gin.Context) {
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
		if errors.Is(err, domain.ErrItemNotInWishlist) {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Error:   "Not Found",
				Message: "Item not found in wishlist",
			})
			return
		}
		h.l.Errorw("Failed to remove item from wishlist", "error", err, "variation_id", variationID)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "Internal Server Error",
			Message: "Failed to remove item from wishlist",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Item removed from wishlist"})
}

// SyncWishlist godoc
// @Summary      Sync anonymous wishlist to user
// @Description  Moves all items from anonymous session (identified by cookie or body) to the authenticated user's wishlist.
// @Tags         Wishlist
// @Accept       json
// @Produce      json
// @Param        lang path string true "Language code (uk, en)"
// @Param        body body SyncWishlistRequest false "Optional session_id to sync from (falls back to cookie)"
// @Security     bearerAuth
// @Success      200  {object} map[string]string
// @Failure      400  {object} ErrorResponse
// @Failure      401  {object} ErrorResponse
// @Failure      500  {object} ErrorResponse
// @Router       /api/{lang}/wishlist/sync [post]
func (h *WishlistHandler) SyncWishlist(c *gin.Context) {
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

	var req SyncWishlistRequest
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
		h.l.Errorw("Failed to sync wishlist", "error", err, "session_id", sessionID, "user_id", userID)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "Internal Server Error",
			Message: "Failed to sync wishlist",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Wishlist synced successfully"})
}
