package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/apiresponse"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware"
)

func TestCatalogV1ListUsesStandardPaginationEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &v1ProductService{products: []domain.Product{
		{ID: uuid.New(), Status: "active"},
		{ID: uuid.New(), Status: "active"},
		{ID: uuid.New(), Status: "active"},
	}}
	renderer := apiresponse.NewErrorRenderer(nil)
	router := gin.New()
	router.Use(renderer.Middleware())
	group := router.Group("/api/v1/catalog/:lang")
	group.Use(middleware.NewLocaleMiddleware(middleware.LocaleOptions{FallbackLocale: "uk", SupportedLocales: []string{"uk"}}))
	RegisterV1Routes(group, service, renderer)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/catalog/uk/products?page=2&limit=2", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	for _, expected := range []string{`"data":[`, `"page":2`, `"limit":2`, `"total":3`, `"total_pages":2`, `"has_previous":true`, `"request_id":"`} {
		if !strings.Contains(body, expected) {
			t.Fatalf("response misses %s: %s", expected, body)
		}
	}
}

func TestCatalogV1ReturnsProblemForMissingProduct(t *testing.T) {
	gin.SetMode(gin.TestMode)
	renderer := apiresponse.NewErrorRenderer(nil)
	router := gin.New()
	router.Use(renderer.Middleware())
	group := router.Group("/api/v1/catalog/:lang")
	group.Use(middleware.NewLocaleMiddleware(middleware.LocaleOptions{FallbackLocale: "uk", SupportedLocales: []string{"uk"}}))
	RegisterV1Routes(group, &v1ProductService{findErr: domain.ErrProductNotFound}, renderer)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/catalog/uk/products/by-slug/missing", nil))

	if recorder.Code != http.StatusNotFound || !strings.Contains(recorder.Body.String(), `"code":"RESOURCE_NOT_FOUND"`) {
		t.Fatalf("unexpected response: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestCatalogV1RejectsUnsafeSlugBeforeService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	renderer := apiresponse.NewErrorRenderer(nil)
	service := &v1ProductService{}
	router := gin.New()
	router.Use(renderer.Middleware())
	group := router.Group("/api/v1/catalog/:lang")
	group.Use(middleware.NewLocaleMiddleware(middleware.LocaleOptions{FallbackLocale: "uk", SupportedLocales: []string{"uk"}}))
	RegisterV1Routes(group, service, renderer)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/catalog/uk/products/by-slug/bad_slug", nil))

	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"code":"INVALID_PAYLOAD"`) || service.findCalls != 0 {
		t.Fatalf("unsafe slug = status %d calls %d body %s", recorder.Code, service.findCalls, recorder.Body.String())
	}
}

func TestCatalogV1RejectsDeepPagination(t *testing.T) {
	gin.SetMode(gin.TestMode)
	renderer := apiresponse.NewErrorRenderer(nil)
	router := gin.New()
	router.Use(renderer.Middleware())
	group := router.Group("/api/v1/catalog/:lang")
	group.Use(middleware.NewLocaleMiddleware(middleware.LocaleOptions{FallbackLocale: "uk", SupportedLocales: []string{"uk"}}))
	RegisterV1Routes(group, &v1ProductService{}, renderer)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/catalog/uk/products?page=1001", nil))

	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"code":"INVALID_PAYLOAD"`) {
		t.Fatalf("deep page must be rejected: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestGroupMediaGroupsRolesAndVariants(t *testing.T) {
	main, variant := uuid.New(), uuid.New()
	variantID := uuid.New()
	got := groupMedia([]domain.ProductMedia{{AssetID: main, Role: "MAIN"}, {AssetID: variant, Role: "GALLERY", VariantID: &variantID}}, map[uuid.UUID]string{main: "https://cdn/main.webp", variant: "https://cdn/variant.webp"})
	if got.Main != "https://cdn/main.webp" || got.Variants[variantID.String()][0] != "https://cdn/variant.webp" {
		t.Fatalf("group=%+v", got)
	}
}

func TestCatalogV1ProductResponseContainsOptionMatrixAndAvailability(t *testing.T) {
	gin.SetMode(gin.TestMode)
	productID, colorID, redID, variantID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	price := mustMoney(1299, "EUR")
	service := &v1ProductService{product: &domain.Product{ID: productID, Status: "active", Options: []domain.ProductOption{{ID: colorID, ProductID: productID, Name: "Color", Values: []domain.ProductOptionValue{{ID: redID, OptionID: colorID, ProductID: productID, Value: "Red"}}}}, Variants: []domain.ProductVariant{{ID: variantID, ProductID: productID, Price: price, OptionValues: []domain.ProductOptionValue{{ID: redID, OptionID: colorID, ProductID: productID}}}}}}
	renderer := apiresponse.NewErrorRenderer(nil)
	router := gin.New()
	router.Use(renderer.Middleware())
	group := router.Group("/api/v1/catalog/:lang")
	group.Use(middleware.NewLocaleMiddleware(middleware.LocaleOptions{FallbackLocale: "uk", SupportedLocales: []string{"uk"}}))
	RegisterV1Routes(group, service, renderer, availabilityFake{values: map[uuid.UUID]bool{variantID: true}})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/catalog/uk/products/by-slug/cream", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	for _, expected := range []string{`"options":[{`, `"name":"Color"`, `"option_value_ids":["` + redID.String(), `"is_available":true`} {
		if !strings.Contains(recorder.Body.String(), expected) {
			t.Fatalf("response misses %s: %s", expected, recorder.Body.String())
		}
	}
}

type v1ProductService struct {
	products  []domain.Product
	findErr   error
	findCalls int
	product   *domain.Product
}

func (s *v1ProductService) Create(context.Context, *domain.Product) error { return nil }
func (s *v1ProductService) FindBySlug(context.Context, string, string) (*domain.Product, error) {
	s.findCalls++
	if s.findErr != nil {
		return nil, s.findErr
	}
	if s.product != nil {
		return s.product, nil
	}
	return nil, domain.ErrProductNotFound
}

type availabilityFake struct {
	values map[uuid.UUID]bool
	err    error
}

func (f availabilityFake) AvailabilityForVariants(context.Context, []uuid.UUID) (map[uuid.UUID]bool, error) {
	return f.values, f.err
}

func mustMoney(amount int64, currency string) money.Money {
	value, err := money.NewMoney(amount, currency)
	if err != nil {
		panic(err)
	}
	return value
}
func (s *v1ProductService) List(context.Context, string) ([]domain.Product, error) {
	return s.products, nil
}
func (s *v1ProductService) ListProducts(_ context.Context, _ string, page, limit int) ([]domain.Product, int64, error) {
	start := (page - 1) * limit
	if start >= len(s.products) {
		return nil, int64(len(s.products)), nil
	}
	end := start + limit
	if end > len(s.products) {
		end = len(s.products)
	}
	return s.products[start:end], int64(len(s.products)), nil
}
