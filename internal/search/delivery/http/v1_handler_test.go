package http

import (
	"context"
	"errors"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/apiresponse"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware"
	search "github.com/VladHrytsaiuk/ecommerce-core/internal/search/domain"
)

func TestSearchV1ParsesFiltersAndReturnsFacets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &searchServiceStub{result: search.ProductSearchResult{Total: 1, Facets: search.Facets{"brand": {"Sony": 1}, "attributes.color": {"black": 1}}}}
	router := searchRouter(service)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodGet, "/api/v1/catalog/uk/search?q=headphones&page=1&limit=10&brand=Sony,Apple&price_min=1000&price_max=2000&in_stock=true&attributes[color]=black", nil))
	if recorder.Code != stdhttp.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if len(service.input.Filters.Brands) != 2 || service.input.Filters.PriceMin == nil || *service.input.Filters.PriceMin != 1000 || service.input.Filters.InStock == nil || !*service.input.Filters.InStock || len(service.input.Filters.Attributes["color"]) != 1 {
		t.Fatalf("parsed input = %#v", service.input)
	}
	for _, expected := range []string{`"facets"`, `"Sony":1`, `"black":1`, `"request_id":"`} {
		if !strings.Contains(recorder.Body.String(), expected) {
			t.Fatalf("response misses %s: %s", expected, recorder.Body.String())
		}
	}
}

func TestSearchV1Returns503WhenProviderIsUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := searchRouter(&searchServiceStub{searchErr: search.ErrUnavailable})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodGet, "/api/v1/catalog/uk/search", nil))
	if recorder.Code != stdhttp.StatusServiceUnavailable || !strings.Contains(recorder.Body.String(), `"code":"SEARCH_UNAVAILABLE"`) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestSearchV1RejectsDeepPagination(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := searchRouter(&searchServiceStub{})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodGet, "/api/v1/catalog/uk/search?page=1001", nil))
	if recorder.Code != stdhttp.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"code":"INVALID_PAYLOAD"`) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestSearchV1RejectsOversizedQueryStringBeforeCallingService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &searchServiceStub{}
	router := searchRouter(service)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodGet, "/api/v1/catalog/uk/search?q="+strings.Repeat("x", maxRawSearchQueryBytes), nil))
	if recorder.Code != stdhttp.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"code":"INVALID_PAYLOAD"`) || service.called {
		t.Fatalf("status=%d called=%t body=%s", recorder.Code, service.called, recorder.Body.String())
	}
}

func searchRouter(service search.SearchService) *gin.Engine {
	renderer := apiresponse.NewErrorRenderer(nil)
	router := gin.New()
	router.Use(renderer.Middleware())
	group := router.Group("/api/v1/catalog/:lang")
	group.Use(middleware.NewLocaleMiddleware(middleware.LocaleOptions{FallbackLocale: "uk", SupportedLocales: []string{"uk"}}))
	RegisterV1Routes(group, service, renderer)
	return router
}

type searchServiceStub struct {
	input     search.SearchProductsInput
	result    search.ProductSearchResult
	searchErr error
	called    bool
}

func (s *searchServiceStub) SearchProducts(_ context.Context, input search.SearchProductsInput) (search.ProductSearchResult, error) {
	s.called = true
	s.input = input
	if s.searchErr != nil {
		return search.ProductSearchResult{}, s.searchErr
	}
	return s.result, nil
}

func (s *searchServiceStub) Autocomplete(context.Context, search.AutocompleteInput) ([]search.Suggestion, error) {
	return nil, errors.New("not implemented")
}
