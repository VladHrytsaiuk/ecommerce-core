package http

import (
	"errors"
	stdhttp "net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/cart/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/cartowner"
)

type Handler struct {
	service       domain.Service
	secureCookies bool
}

func NewHandler(service domain.Service, secureCookies bool) *Handler {
	return &Handler{service: service, secureCookies: secureCookies}
}

type itemRequest struct {
	VariantID uuid.UUID `json:"variant_id" binding:"required"`
	Quantity  int       `json:"quantity" binding:"required,gt=0"`
}

// Get godoc
// @Summary Read the current cart
// @Description Resolves the authenticated customer, or the anonymous buyer's cart_session cookie, issuing one when absent.
// @Tags Cart
// @Produce json
// @Param lang path string true "Locale code"
// @Success 200 {object} domain.Cart
// @Failure 400 {object} map[string]string
// @Router /api/{lang}/cart [get]
func (h *Handler) Get(c *gin.Context) {
	h.respond(c, func(owner domain.Owner) (*domain.Cart, error) { return h.service.GetOrCreate(c, owner) })
}

// Add godoc
// @Summary Add a variant to the cart
// @Tags Cart
// @Accept json
// @Produce json
// @Param lang path string true "Locale code"
// @Param payload body itemRequest true "Cart item"
// @Success 200 {object} domain.Cart
// @Failure 400,422 {object} map[string]string
// @Router /api/{lang}/cart/items [post]
func (h *Handler) Add(c *gin.Context) {
	var request itemRequest
	if c.ShouldBindJSON(&request) != nil {
		c.JSON(stdhttp.StatusBadRequest, gin.H{"error": "invalid cart item"})
		return
	}
	h.respond(c, func(owner domain.Owner) (*domain.Cart, error) {
		return h.service.Add(c, owner, domain.Item{VariantID: request.VariantID, Quantity: request.Quantity})
	})
}

// SetQuantity godoc
// @Summary Replace the quantity of one cart line
// @Tags Cart
// @Accept json
// @Produce json
// @Param lang path string true "Locale code"
// @Param variantID path string true "Product variant UUID"
// @Param payload body itemRequest true "New quantity"
// @Success 200 {object} domain.Cart
// @Failure 400,404,422 {object} map[string]string
// @Router /api/{lang}/cart/items/{variantID} [patch]
func (h *Handler) SetQuantity(c *gin.Context) {
	var request itemRequest
	if c.ShouldBindJSON(&request) != nil {
		c.JSON(stdhttp.StatusBadRequest, gin.H{"error": "invalid cart item"})
		return
	}
	variantID, err := uuid.Parse(c.Param("variantID"))
	if err != nil {
		c.JSON(stdhttp.StatusBadRequest, gin.H{"error": "invalid variant id"})
		return
	}
	request.VariantID = variantID
	h.respond(c, func(owner domain.Owner) (*domain.Cart, error) {
		return h.service.SetQuantity(c, owner, domain.Item{VariantID: request.VariantID, Quantity: request.Quantity})
	})
}

// Remove godoc
// @Summary Remove a variant from the cart
// @Tags Cart
// @Produce json
// @Param lang path string true "Locale code"
// @Param variantID path string true "Product variant UUID"
// @Success 200 {object} domain.Cart
// @Failure 400,404 {object} map[string]string
// @Router /api/{lang}/cart/items/{variantID} [delete]
func (h *Handler) Remove(c *gin.Context) {
	variantID, err := uuid.Parse(c.Param("variantID"))
	if err != nil {
		c.JSON(stdhttp.StatusBadRequest, gin.H{"error": "invalid variant id"})
		return
	}
	h.respond(c, func(owner domain.Owner) (*domain.Cart, error) { return h.service.Remove(c, owner, variantID) })
}
func (h *Handler) respond(c *gin.Context, action func(domain.Owner) (*domain.Cart, error)) {
	owner, created, err := cartowner.FromContext(c)
	if err != nil {
		// The only failure here is an unusable session cookie, and saying so
		// is safe: the value came from the caller.
		c.JSON(stdhttp.StatusBadRequest, gin.H{"error": "cart session is invalid"})
		return
	}
	cart, err := action(owner)
	if err != nil {
		status, message := classifyCartError(err)
		c.JSON(status, gin.H{"error": message})
		return
	}
	if created {
		cartowner.SetSessionCookie(c, *owner.SessionID, h.secureCookies)
	}
	c.JSON(stdhttp.StatusOK, cart)
}

// classifyCartError maps a failure to what the buyer is told.
//
// Anything unrecognized is a 500 with a fixed message. It used to be a 422
// carrying err.Error(): a database failure therefore reached the buyer with
// PostgreSQL's text — which names tables, columns and constraints — under a
// status that tells their client the request was malformed and not worth
// retrying. Both halves were wrong.
func classifyCartError(err error) (int, string) {
	switch {
	case errors.Is(err, domain.ErrItemNotFound):
		return stdhttp.StatusNotFound, "cart item not found"
	case errors.Is(err, domain.ErrInvalidItem):
		return stdhttp.StatusUnprocessableEntity, "cart item is invalid"
	case errors.Is(err, domain.ErrInvalidOwner):
		return stdhttp.StatusUnprocessableEntity, "cart owner is invalid"
	default:
		return stdhttp.StatusInternalServerError, "cart is unavailable"
	}
}
