package http

import (
	"context"
	"errors"
	stdhttp "net/http"
	"strings"
	"unicode"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/apiresponse"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware"
)

// CatalogV1Handler owns the additive, versioned Catalog wire contract. It
// delegates unchanged to the existing Catalog application port.
type CatalogV1Handler struct {
	service      domain.ProductService
	errors       *apiresponse.ErrorRenderer
	availability domain.VariantAvailabilityReader
}

func NewCatalogV1Handler(service domain.ProductService, renderer *apiresponse.ErrorRenderer, readers ...domain.VariantAvailabilityReader) *CatalogV1Handler {
	h := &CatalogV1Handler{service: service, errors: renderer.WithClassifier(classifyCatalogError)}
	if len(readers) > 0 {
		h.availability = readers[0]
	}
	return h
}

type listProductsQuery struct {
	Page  int `form:"page,default=1" binding:"min=1,max=1000"`
	Limit int `form:"limit,default=20" binding:"min=1,max=100"`
}

// List godoc
// @Summary List catalog products (v1)
// @Tags Catalog v1
// @Produce json
// @Param lang path string true "Locale"
// @Param page query int false "Page number" default(1) minimum(1) maximum(1000)
// @Param limit query int false "Page size" default(20) minimum(1) maximum(100)
// @Success 200 {object} apiresponse.PaginatedResponse
// @Failure 400 {object} apiresponse.ProblemDetails
// @Failure 500 {object} apiresponse.ProblemDetails
// @Router /api/v1/catalog/{lang}/products [get]
func (h *CatalogV1Handler) List(c *gin.Context) {
	query := listProductsQuery{Page: 1, Limit: 20}
	if err := c.ShouldBindQuery(&query); err != nil {
		h.errors.Abort(c, apiresponse.InvalidPayload(err))
		return
	}
	products, total, err := h.service.ListProducts(c.Request.Context(), middleware.GetLanguage(c), query.Page, query.Limit)
	if err != nil {
		h.errors.Abort(c, err)
		return
	}
	response := make([]ProductResponse, 0, len(products))
	for index := range products {
		response = append(response, mapProduct(&products[index]))
	}
	totalPages := int((total + int64(query.Limit) - 1) / int64(query.Limit))
	if totalPages == 0 {
		totalPages = 1
	}
	apiresponse.Paginated(c, stdhttp.StatusOK, response, apiresponse.PageMetadata{
		Page:        query.Page,
		Limit:       query.Limit,
		Total:       total,
		TotalPages:  totalPages,
		HasNext:     query.Page < totalPages,
		HasPrevious: query.Page > 1,
	})
}

// GetBySlug godoc
// @Summary Get a catalog product by slug (v1)
// @Tags Catalog v1
// @Produce json
// @Param lang path string true "Locale"
// @Param slug path string true "Localized product slug"
// @Success 200 {object} apiresponse.SuccessResponse
// @Failure 400 {object} apiresponse.ProblemDetails
// @Failure 404 {object} apiresponse.ProblemDetails
// @Router /api/v1/catalog/{lang}/products/by-slug/{slug} [get]
func (h *CatalogV1Handler) GetBySlug(c *gin.Context) {
	slug := c.Param("slug")
	if !validV1Slug(slug) {
		h.errors.Abort(c, apiresponse.InvalidPayload(errors.New("invalid product slug")))
		return
	}
	product, err := h.service.FindBySlug(c.Request.Context(), middleware.GetLanguage(c), slug)
	if err != nil {
		h.errors.Abort(c, err)
		return
	}
	if err := h.hydrateAvailability(c.Request.Context(), product); err != nil {
		h.errors.Abort(c, err)
		return
	}
	apiresponse.Success(c, stdhttp.StatusOK, mapProduct(product))
}

func (h *CatalogV1Handler) hydrateAvailability(ctx context.Context, product *domain.Product) error {
	if product == nil || len(product.Variants) == 0 || h.availability == nil {
		return nil
	}
	ids := make([]uuid.UUID, 0, len(product.Variants))
	for _, variant := range product.Variants {
		ids = append(ids, variant.ID)
	}
	available, err := h.availability.AvailabilityForVariants(ctx, ids)
	if err != nil {
		return err
	}
	for index := range product.Variants {
		product.Variants[index].IsAvailable = available[product.Variants[index].ID]
	}
	return nil
}

// validV1Slug accepts localized letters and digits plus conventional hyphens.
// It rejects path separators, NUL and control characters before the value ever
// reaches a repository; the repository additionally uses parameterized SQL.
func validV1Slug(slug string) bool {
	if slug == "" || len(slug) > 255 || strings.ContainsRune(slug, 0) {
		return false
	}
	for _, value := range slug {
		if !unicode.IsLetter(value) && !unicode.IsDigit(value) && value != '-' {
			return false
		}
	}
	return true
}

func classifyCatalogError(err error) (*apiresponse.PublicError, bool) {
	switch {
	case errors.Is(err, domain.ErrProductNotFound):
		return apiresponse.NotFound(err, "The requested product does not exist."), true
	case errors.Is(err, domain.ErrInvalidProduct):
		return apiresponse.ValidationFailed(err), true
	default:
		return nil, false
	}
}
