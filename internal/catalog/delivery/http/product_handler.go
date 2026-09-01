// Package http exposes the clean Catalog API. It only maps HTTP to the
// application port; persistence and catalog rules stay outside Gin.
package http

import (
	"errors"
	stdhttp "net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware"
)

type ProductHandler struct{ service domain.ProductService }

func NewProductHandler(service domain.ProductService) *ProductHandler {
	return &ProductHandler{service: service}
}

type CreateProductRequest struct {
	CategoryID   *uuid.UUID                  `json:"category_id,omitempty"`
	Status       string                      `json:"status,omitempty"`
	Translations []ProductTranslationRequest `json:"translations" binding:"required,min=1"`
}

type ProductTranslationRequest struct {
	Locale      string `json:"locale" binding:"required"`
	Name        string `json:"name" binding:"required"`
	Description string `json:"description,omitempty"`
	Slug        string `json:"slug" binding:"required"`
}

type ProductResponse struct {
	ID           uuid.UUID                    `json:"id"`
	CategoryID   *uuid.UUID                   `json:"category_id,omitempty"`
	Status       string                       `json:"status"`
	Translations []ProductTranslationResponse `json:"translations"`
	Rating       *ProductRatingResponse       `json:"rating,omitempty"`
	SEO          *domain.ProductSEO           `json:"seo,omitempty"`
	Badges       []domain.ProductBadge        `json:"badges,omitempty"`
	Media        ProductMediaResponse         `json:"media,omitempty"`
	Options      []ProductOptionResponse      `json:"options,omitempty"`
	Variants     []ProductVariantResponse     `json:"variants,omitempty"`
}
type ProductOptionResponse struct {
	ID       uuid.UUID                    `json:"id"`
	Name     string                       `json:"name"`
	Position int                          `json:"position"`
	Values   []ProductOptionValueResponse `json:"values"`
}
type ProductOptionValueResponse struct {
	ID       uuid.UUID `json:"id"`
	Value    string    `json:"value"`
	Position int       `json:"position"`
}
type ProductVariantResponse struct {
	ID             uuid.UUID   `json:"id"`
	SKU            string      `json:"sku,omitempty"`
	Barcode        string      `json:"barcode,omitempty"`
	PriceAmount    int64       `json:"price_amount"`
	Currency       string      `json:"currency"`
	OptionValueIDs []uuid.UUID `json:"option_value_ids"`
	IsAvailable    bool        `json:"is_available"`
}
type ProductMediaResponse struct {
	Main     string              `json:"main,omitempty"`
	Hover    string              `json:"hover,omitempty"`
	Gallery  []string            `json:"gallery,omitempty"`
	Variants map[string][]string `json:"variants,omitempty"`
}

type ProductRatingResponse struct {
	ReviewCount   int     `json:"review_count"`
	AverageRating float64 `json:"average_rating"`
}

type ProductTranslationResponse struct {
	Locale      string `json:"locale"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Slug        string `json:"slug"`
}

func (h *ProductHandler) Create(c *gin.Context) {
	var request CreateProductRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(stdhttp.StatusBadRequest, gin.H{"error": "invalid request", "message": err.Error()})
		return
	}
	product := &domain.Product{CategoryID: request.CategoryID, Status: request.Status, Translations: make([]domain.ProductTranslation, 0, len(request.Translations))}
	for _, translation := range request.Translations {
		product.Translations = append(product.Translations, domain.ProductTranslation{
			Locale: translation.Locale, Name: translation.Name, Description: translation.Description, Slug: translation.Slug,
		})
	}
	if err := h.service.Create(c.Request.Context(), product); err != nil {
		handleProductError(c, err)
		return
	}
	c.JSON(stdhttp.StatusCreated, mapProduct(product))
}

func (h *ProductHandler) GetBySlug(c *gin.Context) {
	product, err := h.service.FindBySlug(c.Request.Context(), middleware.GetLanguage(c), c.Param("slug"))
	if err != nil {
		handleProductError(c, err)
		return
	}
	c.JSON(stdhttp.StatusOK, mapProduct(product))
}

func (h *ProductHandler) List(c *gin.Context) {
	products, err := h.service.List(c.Request.Context(), middleware.GetLanguage(c))
	if err != nil {
		handleProductError(c, err)
		return
	}
	response := make([]ProductResponse, 0, len(products))
	for index := range products {
		response = append(response, mapProduct(&products[index]))
	}
	c.JSON(stdhttp.StatusOK, gin.H{"products": response})
}

func handleProductError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrProductNotFound):
		c.JSON(stdhttp.StatusNotFound, gin.H{"error": "product not found"})
	case errors.Is(err, domain.ErrInvalidProduct):
		c.JSON(stdhttp.StatusBadRequest, gin.H{"error": "invalid product", "message": err.Error()})
	default:
		c.JSON(stdhttp.StatusInternalServerError, gin.H{"error": "internal server error"})
	}
}

func mapProduct(product *domain.Product) ProductResponse {
	translations := make([]ProductTranslationResponse, 0, len(product.Translations))
	for _, translation := range product.Translations {
		translations = append(translations, ProductTranslationResponse{
			Locale: translation.Locale, Name: translation.Name, Description: translation.Description, Slug: translation.Slug,
		})
	}
	response := ProductResponse{ID: product.ID, CategoryID: product.CategoryID, Status: product.Status, Translations: translations}
	if product.Rating != nil {
		response.Rating = &ProductRatingResponse{ReviewCount: product.Rating.ReviewCount, AverageRating: float64(product.Rating.AverageHundredths) / 100}
	}
	response.SEO = product.SEO
	response.Badges = product.Badges
	response.Media = groupMedia(product.Media, nil)
	response.Options = mapProductOptions(product.Options)
	response.Variants = mapProductVariants(product.Variants)
	return response
}

func mapProductOptions(options []domain.ProductOption) []ProductOptionResponse {
	if len(options) == 0 {
		return nil
	}
	result := make([]ProductOptionResponse, 0, len(options))
	for _, option := range options {
		out := ProductOptionResponse{ID: option.ID, Name: option.Name, Position: option.Position, Values: make([]ProductOptionValueResponse, 0, len(option.Values))}
		for _, value := range option.Values {
			out.Values = append(out.Values, ProductOptionValueResponse{ID: value.ID, Value: value.Value, Position: value.Position})
		}
		result = append(result, out)
	}
	return result
}

func mapProductVariants(variants []domain.ProductVariant) []ProductVariantResponse {
	if len(variants) == 0 {
		return nil
	}
	result := make([]ProductVariantResponse, 0, len(variants))
	for _, variant := range variants {
		out := ProductVariantResponse{ID: variant.ID, SKU: variant.SKU, Barcode: variant.Barcode, PriceAmount: variant.Price.Amount(), Currency: variant.Price.Currency(), IsAvailable: variant.IsAvailable, OptionValueIDs: make([]uuid.UUID, 0, len(variant.OptionValues))}
		for _, value := range variant.OptionValues {
			out.OptionValueIDs = append(out.OptionValueIDs, value.ID)
		}
		result = append(result, out)
	}
	return result
}
func groupMedia(links []domain.ProductMedia, urls map[uuid.UUID]string) ProductMediaResponse {
	out := ProductMediaResponse{Variants: map[string][]string{}}
	for _, x := range links {
		u := x.URL
		if u == "" {
			u = urls[x.AssetID]
		}
		if u == "" {
			continue
		}
		if x.VariantID != nil {
			out.Variants[x.VariantID.String()] = append(out.Variants[x.VariantID.String()], u)
			continue
		}
		switch x.Role {
		case "MAIN":
			out.Main = u
		case "HOVER":
			out.Hover = u
		case "GALLERY":
			out.Gallery = append(out.Gallery, u)
		}
	}
	if len(out.Variants) == 0 {
		out.Variants = nil
	}
	return out
}
