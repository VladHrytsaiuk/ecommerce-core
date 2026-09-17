package application

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	search "github.com/VladHrytsaiuk/ecommerce-core/internal/search/domain"
)

func TestSearchProductsBuildsAllowListedMeilisearchFilter(t *testing.T) {
	engine := &searchEngineStub{}
	service, err := NewSearchService(engine)
	if err != nil {
		t.Fatalf("NewSearchService() error = %v", err)
	}
	min, max := int64(1_000), int64(9_000)
	inStock := true
	_, err = service.SearchProducts(context.Background(), search.SearchProductsInput{
		Query: "headphones", Locale: "uk", Page: 2, Limit: 24, Sort: "price_asc",
		Filters: search.Filters{Brands: []string{"Sony", "Apple"}, PriceMin: &min, PriceMax: &max, InStock: &inStock, Attributes: map[string][]string{"color": {"black", "blue"}}},
	})
	if err != nil {
		t.Fatalf("SearchProducts() error = %v", err)
	}
	query := engine.lastQuery
	for _, clause := range []string{
		`locales = "uk"`, `status = "active"`, `brand IN ["Sony", "Apple"]`, `price_min >= 1000`, `price_min <= 9000`, `in_stock = true`, `attributes.color IN ["black", "blue"]`,
	} {
		if !strings.Contains(query.Filter, clause) {
			t.Fatalf("filter %q does not contain %q", query.Filter, clause)
		}
	}
	if got := strings.Join(query.Sort, ","); got != "price_min:asc" || query.Page != 2 || query.Limit != 24 {
		t.Fatalf("normalized query = %#v", query)
	}
}

func TestSearchProductsRejectsUnsafeFilterValues(t *testing.T) {
	service, _ := NewSearchService(&searchEngineStub{})
	_, err := service.SearchProducts(context.Background(), search.SearchProductsInput{Locale: "uk", Filters: search.Filters{Attributes: map[string][]string{"bad.key": {"value"}}}})
	if !errors.Is(err, search.ErrInvalidQuery) {
		t.Fatalf("error = %v, want ErrInvalidQuery", err)
	}
	_, err = service.SearchProducts(context.Background(), search.SearchProductsInput{Locale: "uk", Page: 1001})
	if !errors.Is(err, search.ErrInvalidQuery) {
		t.Fatalf("deep page error = %v, want ErrInvalidQuery", err)
	}
}

func TestSearchProductsRejectsExcessiveFacetInputBeforeCallingProvider(t *testing.T) {
	engine := &searchEngineStub{}
	service, _ := NewSearchService(engine)
	attributes := make(map[string][]string, maxFacetKeys+1)
	for index := 0; index <= maxFacetKeys; index++ {
		attributes["facet"+strconv.Itoa(index)] = []string{"value"}
	}
	_, err := service.SearchProducts(context.Background(), search.SearchProductsInput{Locale: "uk", Filters: search.Filters{Attributes: attributes}})
	if !errors.Is(err, search.ErrInvalidQuery) || engine.called {
		t.Fatalf("attribute overflow error=%v engineCalled=%t", err, engine.called)
	}

	tooManyValues := make([]string, maxFacetValuesTotal+1)
	for index := range tooManyValues {
		tooManyValues[index] = "brand" + strconv.Itoa(index)
	}
	_, err = service.SearchProducts(context.Background(), search.SearchProductsInput{Locale: "uk", Filters: search.Filters{Brands: tooManyValues}})
	if !errors.Is(err, search.ErrInvalidQuery) || engine.called {
		t.Fatalf("values overflow error=%v engineCalled=%t", err, engine.called)
	}
	withinPerFacetLimit := make([]string, maxFacetValuesPerKey)
	for index := range withinPerFacetLimit {
		withinPerFacetLimit[index] = "value" + strconv.Itoa(index)
	}
	_, err = service.SearchProducts(context.Background(), search.SearchProductsInput{
		Locale: "uk",
		Filters: search.Filters{
			Brands: withinPerFacetLimit,
			Attributes: map[string][]string{
				"color": append([]string(nil), withinPerFacetLimit[:26]...),
				"size":  append([]string(nil), withinPerFacetLimit[:26]...),
			},
		},
	})
	if !errors.Is(err, search.ErrInvalidQuery) || engine.called {
		t.Fatalf("total values overflow error=%v engineCalled=%t", err, engine.called)
	}

	_, err = service.SearchProducts(context.Background(), search.SearchProductsInput{Locale: "uk", Query: strings.Repeat("q", maxQueryRunes+1)})
	if !errors.Is(err, search.ErrInvalidQuery) || engine.called {
		t.Fatalf("query overflow error=%v engineCalled=%t", err, engine.called)
	}
}

func TestSearchProductsMapsProviderFailureToUnavailable(t *testing.T) {
	service, _ := NewSearchService(&searchEngineStub{searchErr: errors.New("dial refused")})
	_, err := service.SearchProducts(context.Background(), search.SearchProductsInput{Locale: "uk"})
	if !errors.Is(err, search.ErrUnavailable) {
		t.Fatalf("error = %v, want ErrUnavailable", err)
	}
}

type searchEngineStub struct {
	lastQuery search.ProductSearchQuery
	searchErr error
	called    bool
}

func (s *searchEngineStub) Search(_ context.Context, query search.ProductSearchQuery) (search.ProductSearchResult, error) {
	s.called = true
	s.lastQuery = query
	return search.ProductSearchResult{}, s.searchErr
}

func (s *searchEngineStub) Autocomplete(_ context.Context, query search.ProductSearchQuery) ([]search.Suggestion, error) {
	s.called = true
	s.lastQuery = query
	return nil, s.searchErr
}
