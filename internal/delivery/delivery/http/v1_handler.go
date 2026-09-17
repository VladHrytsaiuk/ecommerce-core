// Package http exposes provider-neutral delivery selectors over HTTP.
package http

import (
	"errors"
	stdhttp "net/http"

	"github.com/gin-gonic/gin"

	deliveryApp "github.com/VladHrytsaiuk/ecommerce-core/internal/delivery/application"
	deliveryDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/delivery/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/apiresponse"
)

type LocationV1Handler struct {
	service *deliveryApp.LocationService
	errors  *apiresponse.ErrorRenderer
}

func NewLocationV1Handler(service *deliveryApp.LocationService, renderer *apiresponse.ErrorRenderer) *LocationV1Handler {
	return &LocationV1Handler{service: service, errors: renderer.WithClassifier(classifyLocationError)}
}

// Areas godoc
// @Summary List delivery areas (v1)
// @Tags Delivery v1
// @Produce json
// @Param provider path string true "Enabled delivery provider"
// @Success 200 {object} apiresponse.SuccessResponse
// @Failure 400 {object} apiresponse.ProblemDetails
// @Failure 503 {object} apiresponse.ProblemDetails
// @Router /api/v1/delivery/{provider}/areas [get]
func (h *LocationV1Handler) Areas(c *gin.Context) {
	areas, err := h.service.Areas(c.Request.Context(), c.Param("provider"))
	if err != nil {
		h.errors.Abort(c, err)
		return
	}
	apiresponse.Success(c, stdhttp.StatusOK, areas)
}

// Cities godoc
// @Summary List delivery cities for an area (v1)
// @Tags Delivery v1
// @Produce json
// @Param provider path string true "Enabled delivery provider"
// @Param area_id query string true "Provider area ID"
// @Success 200 {object} apiresponse.SuccessResponse
// @Failure 400 {object} apiresponse.ProblemDetails
// @Failure 503 {object} apiresponse.ProblemDetails
// @Router /api/v1/delivery/{provider}/cities [get]
func (h *LocationV1Handler) Cities(c *gin.Context) {
	cities, err := h.service.Cities(c.Request.Context(), c.Param("provider"), c.Query("area_id"))
	if err != nil {
		h.errors.Abort(c, err)
		return
	}
	apiresponse.Success(c, stdhttp.StatusOK, cities)
}

type servicePointsQuery struct {
	CityID string `form:"city_id" binding:"required,max=128"`
	Kind   string `form:"kind" binding:"omitempty,oneof=branch postomat cargo"`
	Page   int    `form:"page,default=1" binding:"min=1,max=1000"`
	Limit  int    `form:"limit,default=20" binding:"min=1,max=100"`
}

// ServicePoints godoc
// @Summary List delivery service points (v1)
// @Tags Delivery v1
// @Produce json
// @Param provider path string true "Enabled delivery provider"
// @Param city_id query string true "Provider city ID"
// @Param kind query string false "branch, postomat, or cargo"
// @Param page query int false "Page number" default(1) minimum(1) maximum(1000)
// @Param limit query int false "Page size" default(20) minimum(1) maximum(100)
// @Success 200 {object} apiresponse.SuccessResponse
// @Failure 400 {object} apiresponse.ProblemDetails
// @Failure 503 {object} apiresponse.ProblemDetails
// @Router /api/v1/delivery/{provider}/service-points [get]
func (h *LocationV1Handler) ServicePoints(c *gin.Context) {
	query := servicePointsQuery{Page: 1, Limit: 20}
	if err := c.ShouldBindQuery(&query); err != nil {
		h.errors.Abort(c, apiresponse.InvalidPayload(err))
		return
	}
	result, err := h.service.ServicePoints(c.Request.Context(), c.Param("provider"), deliveryDomain.ServicePointQuery{CityID: query.CityID, Kind: query.Kind, Page: query.Page, Limit: query.Limit})
	if err != nil {
		h.errors.Abort(c, err)
		return
	}
	apiresponse.Success(c, stdhttp.StatusOK, result)
}

func RegisterV1LocationRoutes(group *gin.RouterGroup, service *deliveryApp.LocationService, renderer *apiresponse.ErrorRenderer) {
	if group == nil || service == nil || renderer == nil {
		return
	}
	handler := NewLocationV1Handler(service, renderer)
	group.GET("/:provider/areas", handler.Areas)
	group.GET("/:provider/cities", handler.Cities)
	group.GET("/:provider/service-points", handler.ServicePoints)
}

func classifyLocationError(err error) (*apiresponse.PublicError, bool) {
	switch {
	case errors.Is(err, deliveryDomain.ErrInvalidLocationQuery):
		return apiresponse.InvalidPayload(err), true
	case errors.Is(err, deliveryDomain.ErrLocationProviderUnavailable):
		return apiresponse.Unavailable(err), true
	case errors.Is(err, deliveryDomain.ErrProviderUnavailable):
		return apiresponse.Unavailable(err), true
	default:
		return nil, false
	}
}
