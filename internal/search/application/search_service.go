package application

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	search "github.com/VladHrytsaiuk/ecommerce-core/internal/search/domain"
)

const (
	defaultPage          = 1
	defaultLimit         = 24
	maxPage              = 1000
	maxLimit             = 100
	maxQueryRunes        = 256
	maxFacetKeys         = 12
	maxFacetValuesPerKey = 50
	maxFacetValuesTotal  = 100
)

type SearchService struct{ engine search.ProductSearchEngine }

func NewSearchService(engine search.ProductSearchEngine) (*SearchService, error) {
	if engine == nil {
		return nil, fmt.Errorf("search engine is required")
	}
	return &SearchService{engine: engine}, nil
}

func (s *SearchService) SearchProducts(ctx context.Context, input search.SearchProductsInput) (search.ProductSearchResult, error) {
	query, err := normalizeSearch(input)
	if err != nil {
		return search.ProductSearchResult{}, err
	}
	result, err := s.engine.Search(ctx, query)
	if err != nil {
		return search.ProductSearchResult{}, fmt.Errorf("%w: %v", search.ErrUnavailable, err)
	}
	return result, nil
}

func (s *SearchService) Autocomplete(ctx context.Context, input search.AutocompleteInput) ([]search.Suggestion, error) {
	query := strings.TrimSpace(input.Query)
	if strings.TrimSpace(input.Locale) == "" || query == "" {
		return nil, fmt.Errorf("%w: locale and query are required", search.ErrInvalidQuery)
	}
	if utf8.RuneCountInString(query) > maxQueryRunes {
		return nil, fmt.Errorf("%w: query exceeds %d characters", search.ErrInvalidQuery, maxQueryRunes)
	}
	limit := input.Limit
	if limit == 0 {
		limit = 8
	}
	if limit < 1 || limit > 20 {
		return nil, fmt.Errorf("%w: suggestion limit must be between 1 and 20", search.ErrInvalidQuery)
	}
	result, err := s.engine.Autocomplete(ctx, search.ProductSearchQuery{Query: query, Locale: strings.ToLower(strings.TrimSpace(input.Locale)), Page: 1, Limit: limit, Filter: localeFilter(input.Locale)})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", search.ErrUnavailable, err)
	}
	return result, nil
}

func normalizeSearch(input search.SearchProductsInput) (search.ProductSearchQuery, error) {
	locale := strings.ToLower(strings.TrimSpace(input.Locale))
	if locale == "" {
		return search.ProductSearchQuery{}, fmt.Errorf("%w: locale is required", search.ErrInvalidQuery)
	}
	page, limit := input.Page, input.Limit
	if page == 0 {
		page = defaultPage
	}
	if limit == 0 {
		limit = defaultLimit
	}
	if page < 1 || page > maxPage || limit < 1 || limit > maxLimit {
		return search.ProductSearchQuery{}, fmt.Errorf("%w: page or limit is out of range", search.ErrInvalidQuery)
	}
	query := strings.TrimSpace(input.Query)
	if utf8.RuneCountInString(query) > maxQueryRunes {
		return search.ProductSearchQuery{}, fmt.Errorf("%w: query exceeds %d characters", search.ErrInvalidQuery, maxQueryRunes)
	}
	if input.Filters.PriceMin != nil && *input.Filters.PriceMin < 0 || input.Filters.PriceMax != nil && *input.Filters.PriceMax < 0 {
		return search.ProductSearchQuery{}, fmt.Errorf("%w: prices must not be negative", search.ErrInvalidQuery)
	}
	if input.Filters.PriceMin != nil && input.Filters.PriceMax != nil && *input.Filters.PriceMin > *input.Filters.PriceMax {
		return search.ProductSearchQuery{}, fmt.Errorf("%w: price_min must not exceed price_max", search.ErrInvalidQuery)
	}
	if err := validateFacetLimits(input.Filters); err != nil {
		return search.ProductSearchQuery{}, err
	}
	filter, err := BuildFilter(locale, input.Filters)
	if err != nil {
		return search.ProductSearchQuery{}, err
	}
	sort, err := mapSort(input.Sort)
	if err != nil {
		return search.ProductSearchQuery{}, err
	}
	return search.ProductSearchQuery{Query: query, Locale: locale, Page: page, Limit: limit, Sort: sort, Filter: filter}, nil
}

func validateFacetLimits(filters search.Filters) error {
	if len(filters.Brands) > maxFacetValuesPerKey || len(filters.Attributes) > maxFacetKeys {
		return fmt.Errorf("%w: too many facet filters", search.ErrInvalidQuery)
	}
	total := len(filters.Brands)
	for _, values := range filters.Attributes {
		if len(values) > maxFacetValuesPerKey {
			return fmt.Errorf("%w: too many values for one facet", search.ErrInvalidQuery)
		}
		total += len(values)
		if total > maxFacetValuesTotal {
			return fmt.Errorf("%w: too many facet values", search.ErrInvalidQuery)
		}
	}
	return nil
}

// BuildFilter emits Meilisearch filter syntax exclusively from typed values.
// The public HTTP layer never accepts a raw filter expression.
func BuildFilter(locale string, filters search.Filters) (string, error) {
	clauses := []string{localeFilter(locale), `status = "active"`}
	if len(filters.Brands) > 0 {
		values, err := quotedValues(filters.Brands)
		if err != nil {
			return "", err
		}
		clauses = append(clauses, "brand IN ["+strings.Join(values, ", ")+"]")
	}
	if filters.PriceMin != nil {
		clauses = append(clauses, "price_min >= "+strconv.FormatInt(*filters.PriceMin, 10))
	}
	if filters.PriceMax != nil {
		clauses = append(clauses, "price_min <= "+strconv.FormatInt(*filters.PriceMax, 10))
	}
	if filters.InStock != nil {
		clauses = append(clauses, "in_stock = "+strconv.FormatBool(*filters.InStock))
	}
	for key, values := range filters.Attributes {
		if !validAttributeKey(key) {
			return "", fmt.Errorf("%w: invalid attribute key", search.ErrInvalidQuery)
		}
		quoted, err := quotedValues(values)
		if err != nil {
			return "", err
		}
		clauses = append(clauses, "attributes."+key+" IN ["+strings.Join(quoted, ", ")+"]")
	}
	return strings.Join(clauses, " AND "), nil
}

func localeFilter(locale string) string {
	return `locales = "` + escape(strings.ToLower(strings.TrimSpace(locale))) + `"`
}

func quotedValues(values []string) ([]string, error) {
	if len(values) == 0 || len(values) > maxFacetValuesPerKey {
		return nil, fmt.Errorf("%w: filter values must contain 1 to %d values", search.ErrInvalidQuery, maxFacetValuesPerKey)
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || len(value) > 128 {
			return nil, fmt.Errorf("%w: invalid filter value", search.ErrInvalidQuery)
		}
		result = append(result, `"`+escape(value)+`"`)
	}
	return result, nil
}

func escape(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	return strings.ReplaceAll(value, `"`, `\"`)
}

func validAttributeKey(value string) bool {
	if len(value) == 0 || len(value) > 64 {
		return false
	}
	for index, char := range value {
		if (char < 'a' || char > 'z') && (index == 0 || (char < '0' || char > '9') && char != '_') {
			return false
		}
	}
	return true
}

func mapSort(value string) ([]string, error) {
	switch strings.TrimSpace(value) {
	case "", "relevance":
		return nil, nil
	case "price_asc":
		return []string{"price_min:asc"}, nil
	case "price_desc":
		return []string{"price_min:desc"}, nil
	case "newest":
		return []string{"updated_at:desc"}, nil
	default:
		return nil, fmt.Errorf("%w: unsupported sort", search.ErrInvalidQuery)
	}
}

var _ search.SearchService = (*SearchService)(nil)
