package http

import (
	stdhttp "net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
)

type VariantHandler struct {
	service domain.VariantService
}

func NewVariantHandler(service domain.VariantService) *VariantHandler {
	return &VariantHandler{service: service}
}

type CreateVariantRequest struct {
	ProductID   uuid.UUID `json:"product_id" binding:"required"`
	SKU         string    `json:"sku,omitempty"`
	Barcode     string    `json:"barcode,omitempty"`
	Status      string    `json:"status,omitempty"`
	PriceAmount int64     `json:"price_amount" binding:"gte=0"`
	WeightGrams int       `json:"weight_grams" binding:"gte=0"`
}

type VariantResponse struct {
	ID          uuid.UUID `json:"id"`
	ProductID   uuid.UUID `json:"product_id"`
	SKU         string    `json:"sku,omitempty"`
	Barcode     string    `json:"barcode,omitempty"`
	Status      string    `json:"status"`
	PriceAmount int64     `json:"price_amount"`
	Currency    string    `json:"currency"`
	WeightGrams int       `json:"weight_grams"`
}

func (h *VariantHandler) Create(c *gin.Context) {
	var request CreateVariantRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(stdhttp.StatusBadRequest, gin.H{"error": "invalid request", "message": err.Error()})
		return
	}
	variant := &domain.ProductVariant{
		ProductID:   request.ProductID,
		SKU:         request.SKU,
		Barcode:     request.Barcode,
		Status:      request.Status,
		Price:       money.Money{Amount: request.PriceAmount, Currency: ""},
		WeightGrams: request.WeightGrams,
	}
	if err := h.service.Create(c.Request.Context(), variant); err != nil {
		handleProductError(c, err)
		return
	}
	c.JSON(stdhttp.StatusCreated, mapVariant(variant))
}

func mapVariant(variant *domain.ProductVariant) VariantResponse {
	return VariantResponse{ID: variant.ID, ProductID: variant.ProductID, SKU: variant.SKU, Barcode: variant.Barcode, Status: variant.Status, PriceAmount: variant.Price.Amount, Currency: variant.Price.Currency, WeightGrams: variant.WeightGrams}
}
