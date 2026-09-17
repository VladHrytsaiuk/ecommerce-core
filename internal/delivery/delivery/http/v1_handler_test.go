package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	deliveryApp "github.com/VladHrytsaiuk/ecommerce-core/internal/delivery/application"
	deliveryDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/delivery/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/apiresponse"
)

func TestLocationV1RoutesValidateAndRenderLocations(t *testing.T) {
	gin.SetMode(gin.TestMode)
	registry, err := deliveryApp.NewRegistry([]string{"fake"}, "fake", locationCarrier{})
	if err != nil {
		t.Fatal(err)
	}
	renderer := apiresponse.NewErrorRenderer(nil)
	router := gin.New()
	router.Use(renderer.Middleware())
	RegisterV1LocationRoutes(router.Group("/api/v1/delivery"), deliveryApp.NewLocationService(registry), renderer)

	valid := httptest.NewRecorder()
	router.ServeHTTP(valid, httptest.NewRequest(http.MethodGet, "/api/v1/delivery/fake/service-points?city_id=city-1&kind=branch", nil))
	if valid.Code != http.StatusOK {
		t.Fatalf("valid status = %d, body = %s", valid.Code, valid.Body.String())
	}
	invalid := httptest.NewRecorder()
	router.ServeHTTP(invalid, httptest.NewRequest(http.MethodGet, "/api/v1/delivery/fake/service-points?city_id=city-1&limit=101", nil))
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid status = %d, body = %s", invalid.Code, invalid.Body.String())
	}
}

type locationCarrier struct{}

func (locationCarrier) Code() string { return "fake" }
func (locationCarrier) Quote(context.Context, deliveryDomain.ShipmentQuoteRequest) ([]deliveryDomain.ShippingOption, error) {
	return nil, nil
}
func (locationCarrier) CreateShipment(context.Context, deliveryDomain.CreateShipmentRequest) (deliveryDomain.ShipmentResult, error) {
	return deliveryDomain.ShipmentResult{}, nil
}
func (locationCarrier) Track(context.Context, deliveryDomain.TrackingRequest) (deliveryDomain.TrackingResult, error) {
	return deliveryDomain.TrackingResult{}, nil
}
func (locationCarrier) ListAreas(context.Context) ([]deliveryDomain.Area, error) {
	return []deliveryDomain.Area{{ID: "area-1", Name: "Area"}}, nil
}
func (locationCarrier) ListCities(context.Context, string) ([]deliveryDomain.City, error) {
	return []deliveryDomain.City{{ID: "city-1", AreaID: "area-1", Name: "City"}}, nil
}
func (locationCarrier) ListServicePoints(_ context.Context, query deliveryDomain.ServicePointQuery) (deliveryDomain.ServicePointPage, error) {
	return deliveryDomain.ServicePointPage{Items: []deliveryDomain.ServicePoint{{ID: "branch-1", Name: "Branch", Kind: query.Kind}}, Page: query.Page, Limit: query.Limit, Total: 1}, nil
}
