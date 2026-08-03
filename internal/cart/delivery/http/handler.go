package http

import (
	stdhttp "net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/cart/domain"
)

const sessionCookie = "cart_session"

type Handler struct{ service domain.Service }

func NewHandler(service domain.Service) *Handler { return &Handler{service: service} }

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
	owner, created, err := ownerFor(c)
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
		c.SetCookie(sessionCookie, owner.SessionID.String(), 60*60*24*30, "/", "", false, true)
	}
	c.JSON(stdhttp.StatusOK, cart)
}
func ownerFor(c *gin.Context) (domain.Owner, bool, error) {
	if raw, ok := c.Get("user_id"); ok {
		if id, ok := raw.(uuid.UUID); ok && id != uuid.Nil {
			return domain.Owner{CustomerID: &id}, false, nil
		}
	}
	if raw, err := c.Cookie(sessionCookie); err == nil {
		id, parseErr := uuid.Parse(raw)
		if parseErr == nil {
			return domain.Owner{SessionID: &id}, false, nil
		}
	}
	id := uuid.New()
	return domain.Owner{SessionID: &id}, true, nil
}
