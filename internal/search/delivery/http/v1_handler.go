// Package http exposes the versioned public Search transport contract.
package http

import (
	"errors"
	"fmt"
	stdhttp "net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/apiresponse"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware"
	search "github.com/VladHrytsaiuk/ecommerce-core/internal/search/domain"
)

const maxRawSearchQueryBytes = 8 * 1024

type SearchV1Handler struct {
	service search.SearchService
	errors  *apiresponse.ErrorRenderer
}

func NewSearchV1Handler(service search.SearchService, renderer *apiresponse.ErrorRenderer) *SearchV1Handler {
	return &SearchV1Handler{service: service, errors: renderer.WithClassifier(classifySearchError)}
}

func RegisterV1Routes(group *gin.RouterGroup, service search.SearchService, renderer *apiresponse.ErrorRenderer) {
	if group == nil || service == nil || renderer == nil {
		return
	}
	handler := NewSearchV1Handler(service, renderer)
	group.GET("/search", handler.Search)
	group.GET("/suggestions", handler.Autocomplete)
}

// Search godoc
// @Summary Search products with filters and facets (v1)
// @Tags Search v1
// @Produce json
// @Param lang path string true "Locale"
// @Param q query string false "Full-text query"
// @Param page query int false "Page number" default(1) minimum(1) maximum(1000)
// @Param limit query int false "Page size" default(24) minimum(1) maximum(100)
// @Param sort query string false "relevance, price_asc, price_desc, newest"
// @Param brand query []string false "Brand filter; repeat or comma-separate values"
// @Param price_min query int false "Minimum price in minor currency units"
// @Param price_max query int false "Maximum price in minor currency units"
// @Param in_stock query bool false "Only in-stock products"
// @Param attributes[color] query []string false "Facet attribute values"
// @Success 200 {object} apiresponse.PaginatedResponse
// @Failure 400,503 {object} apiresponse.ProblemDetails
// @Router /api/v1/catalog/{lang}/search [get]
func (h *SearchV1Handler) Search(c *gin.Context) {
	input, err := parseSearchInput(c)
	if err != nil {
		h.errors.Abort(c, err)
		return
	}
	result, err := h.service.SearchProducts(c.Request.Context(), input)
	if err != nil {
		h.errors.Abort(c, err)
		return
	}
	totalPages := int((result.Total + int64(input.Limit) - 1) / int64(input.Limit))
	if totalPages == 0 {
		totalPages = 1
	}
	apiresponse.Paginated(c, stdhttp.StatusOK, result.Documents, apiresponse.PageMetadata{
		Page: input.Page, Limit: input.Limit, Total: result.Total, TotalPages: totalPages,
		HasNext: input.Page < totalPages, HasPrevious: input.Page > 1, Facets: result.Facets,
	})
}

// Autocomplete godoc
// @Summary Autocomplete product names (v1)
// @Tags Search v1
// @Produce json
// @Param lang path string true "Locale"
// @Param q query string true "Search prefix"
// @Param limit query int false "Suggestion limit" default(8) minimum(1) maximum(20)
// @Success 200 {object} apiresponse.SuccessResponse
// @Failure 400,503 {object} apiresponse.ProblemDetails
// @Router /api/v1/catalog/{lang}/suggestions [get]
func (h *SearchV1Handler) Autocomplete(c *gin.Context) {
	if err := validateRawSearchQuery(c); err != nil {
		h.errors.Abort(c, err)
		return
	}
	limit, err := queryInt(c, "limit", 8)
	if err != nil {
		h.errors.Abort(c, err)
		return
	}
	if limit < 1 || limit > 20 {
		h.errors.Abort(c, invalidQuery("limit"))
		return
	}
	suggestions, err := h.service.Autocomplete(c.Request.Context(), search.AutocompleteInput{Query: c.Query("q"), Locale: middleware.GetLanguage(c), Limit: limit})
	if err != nil {
		h.errors.Abort(c, err)
		return
	}
	apiresponse.Success(c, stdhttp.StatusOK, suggestions)
}

func parseSearchInput(c *gin.Context) (search.SearchProductsInput, error) {
	if err := validateRawSearchQuery(c); err != nil {
		return search.SearchProductsInput{}, err
	}
	page, err := queryInt(c, "page", 1)
	if err != nil {
		return search.SearchProductsInput{}, err
	}
	limit, err := queryInt(c, "limit", 24)
	if err != nil {
		return search.SearchProductsInput{}, err
	}
	if page < 1 || page > 1000 || limit < 1 || limit > 100 {
		return search.SearchProductsInput{}, invalidQuery("page or limit")
	}
	priceMin, err := queryInt64(c, "price_min")
	if err != nil {
		return search.SearchProductsInput{}, err
	}
	priceMax, err := queryInt64(c, "price_max")
	if err != nil {
		return search.SearchProductsInput{}, err
	}
	var inStock *bool
	if raw, ok := c.GetQuery("in_stock"); ok {
		parsed, parseErr := strconv.ParseBool(raw)
		if parseErr != nil {
			return search.SearchProductsInput{}, invalidQuery("in_stock")
		}
		inStock = &parsed
	}
	return search.SearchProductsInput{
		Query: c.Query("q"), Locale: middleware.GetLanguage(c), Page: page, Limit: limit, Sort: c.Query("sort"),
		Filters: search.Filters{Brands: splitValues(c.QueryArray("brand")), PriceMin: priceMin, PriceMax: priceMax, InStock: inStock, Attributes: parseAttributes(c)},
	}, nil
}

func validateRawSearchQuery(c *gin.Context) error {
	if len(c.Request.URL.RawQuery) > maxRawSearchQueryBytes {
		return invalidQuery("query string is too large")
	}
	return nil
}

func queryInt(c *gin.Context, key string, fallback int) (int, error) {
	raw, ok := c.GetQuery(key)
	if !ok || raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, invalidQuery(key)
	}
	return value, nil
}

func queryInt64(c *gin.Context, key string) (*int64, error) {
	raw, ok := c.GetQuery(key)
	if !ok || raw == "" {
		return nil, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return nil, invalidQuery(key)
	}
	return &value, nil
}

func parseAttributes(c *gin.Context) map[string][]string {
	attributes := make(map[string][]string)
	for key, values := range c.Request.URL.Query() {
		if !strings.HasPrefix(key, "attributes[") || !strings.HasSuffix(key, "]") {
			continue
		}
		name := strings.TrimSuffix(strings.TrimPrefix(key, "attributes["), "]")
		attributes[name] = append(attributes[name], splitValues(values)...)
	}
	return attributes
}

func splitValues(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			if part = strings.TrimSpace(part); part != "" {
				result = append(result, part)
			}
		}
	}
	return result
}

func invalidQuery(field string) error {
	return fmt.Errorf("%w: invalid %s", search.ErrInvalidQuery, field)
}

func classifySearchError(err error) (*apiresponse.PublicError, bool) {
	switch {
	case errors.Is(err, search.ErrInvalidQuery):
		return apiresponse.InvalidPayload(err), true
	case errors.Is(err, search.ErrUnavailable):
		return apiresponse.SearchUnavailable(err), true
	default:
		return nil, false
	}
}
