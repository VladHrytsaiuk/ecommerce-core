package http

import (
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

func (h *Handler) Get(c *gin.Context) {
	h.respond(c, func(owner domain.Owner) (*domain.Cart, error) { return h.service.GetOrCreate(c, owner) })
}
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
		c.JSON(stdhttp.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	cart, err := action(owner)
	if err != nil {
		status := stdhttp.StatusUnprocessableEntity
		if err == domain.ErrItemNotFound {
			status = stdhttp.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	if created {
		cartowner.SetSessionCookie(c, *owner.SessionID, h.secureCookies)
	}
	c.JSON(stdhttp.StatusOK, cart)
}
