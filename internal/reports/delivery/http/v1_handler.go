// Package http exposes the read-only Reports Admin API.
package http

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	adminDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/apiresponse"
	shared "github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware"
	reportsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/reports/domain"
)

const (
	PermissionReportsRead    = "reports:read"
	PermissionReportsRebuild = "reports:rebuild"
)

type ReportsV1AdminHandler struct {
	service reportsDomain.QueryService
	rebuild reportsDomain.Rebuilder
	errors  *apiresponse.ErrorRenderer
}

func NewReportsV1AdminHandler(service reportsDomain.QueryService, rebuild reportsDomain.Rebuilder, renderer *apiresponse.ErrorRenderer) *ReportsV1AdminHandler {
	return &ReportsV1AdminHandler{service: service, rebuild: rebuild, errors: renderer}
}

type dateRangeQuery struct {
	From     string `form:"from" binding:"required,len=10"`
	To       string `form:"to" binding:"required,len=10"`
	Currency string `form:"currency" binding:"required,len=3"`
	Limit    int    `form:"limit" binding:"omitempty,min=1,max=100"`
}

// Revenue godoc
// @Summary Get aggregated revenue (v1 admin)
// @Tags Reports v1
// @Produce json
// @Param from query string true "Inclusive ISO date" example(2026-01-01)
// @Param to query string true "Inclusive ISO date" example(2026-01-31)
// @Param currency query string true "ISO 4217 currency" example(UAH)
// @Success 200 {object} apiresponse.SuccessResponse
// @Failure 400,401,403 {object} apiresponse.ProblemDetails
// @Router /api/v1/admin/reports/revenue [get]
func (h *ReportsV1AdminHandler) Revenue(c *gin.Context) {
	query, period, ok := h.query(c, false)
	if !ok {
		return
	}
	summary, err := h.service.Revenue(c.Request.Context(), period, query.Currency)
	if err != nil {
		h.errors.Abort(c, apiresponse.InvalidPayload(err))
		return
	}
	apiresponse.Success(c, http.StatusOK, summary)
}

// TopProducts godoc
// @Summary Get top-selling products (v1 admin)
// @Tags Reports v1
// @Produce json
// @Param from query string true "Inclusive ISO date" example(2026-01-01)
// @Param to query string true "Inclusive ISO date" example(2026-01-31)
// @Param currency query string true "ISO 4217 currency" example(UAH)
// @Param limit query int false "Results limit" default(10) minimum(1) maximum(100)
// @Success 200 {object} apiresponse.SuccessResponse
// @Failure 400,401,403 {object} apiresponse.ProblemDetails
// @Router /api/v1/admin/reports/products/top [get]
func (h *ReportsV1AdminHandler) TopProducts(c *gin.Context) {
	query, period, ok := h.query(c, true)
	if !ok {
		return
	}
	products, err := h.service.TopProducts(c.Request.Context(), period, query.Currency, query.Limit)
	if err != nil {
		h.errors.Abort(c, apiresponse.InvalidPayload(err))
		return
	}
	apiresponse.Success(c, http.StatusOK, products)
}

// Funnel godoc
// @Summary Get daily sales funnel (v1 admin)
// @Tags Reports v1
// @Produce json
// @Param from query string true "Inclusive ISO date" example(2026-01-01)
// @Param to query string true "Inclusive ISO date" example(2026-01-31)
// @Success 200 {object} apiresponse.SuccessResponse
// @Failure 400,401,403 {object} apiresponse.ProblemDetails
// @Router /api/v1/admin/reports/funnel [get]
func (h *ReportsV1AdminHandler) Funnel(c *gin.Context) {
	var query struct {
		From string `form:"from" binding:"required,len=10"`
		To   string `form:"to" binding:"required,len=10"`
	}
	if err := c.ShouldBindQuery(&query); err != nil {
		h.errors.Abort(c, apiresponse.InvalidPayload(err))
		return
	}
	from, err := time.Parse("2006-01-02", query.From)
	if err != nil {
		h.errors.Abort(c, apiresponse.InvalidPayload(err))
		return
	}
	to, err := time.Parse("2006-01-02", query.To)
	if err != nil {
		h.errors.Abort(c, apiresponse.InvalidPayload(err))
		return
	}
	period := reportsDomain.DateRange{From: from, To: to}
	funnel, err := h.service.Funnel(c.Request.Context(), period)
	if err != nil {
		h.errors.Abort(c, apiresponse.InvalidPayload(err))
		return
	}
	apiresponse.Success(c, http.StatusOK, funnel)
}

type rebuildRequest struct {
	From string `json:"from" binding:"required,len=10"`
	To   string `json:"to" binding:"required,len=10"`
}

// Rebuild godoc
// @Summary Rebuild reports projections (v1 admin)
// @Tags Reports v1
// @Accept json
// @Produce json
// @Param body body rebuildRequest true "Inclusive ISO date range"
// @Success 202 {object} apiresponse.SuccessResponse
// @Failure 400,401,403,409 {object} apiresponse.ProblemDetails
// @Router /api/v1/admin/reports/rebuild [post]
func (h *ReportsV1AdminHandler) Rebuild(c *gin.Context) {
	if h.rebuild == nil {
		h.errors.Abort(c, apiresponse.Unavailable(nil))
		return
	}
	var request rebuildRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.errors.Abort(c, apiresponse.InvalidPayload(err))
		return
	}
	period, err := parseDateRange(request.From, request.To)
	if err != nil {
		h.errors.Abort(c, apiresponse.InvalidPayload(err))
		return
	}
	started, err := h.rebuild.StartAsync(period)
	if err != nil {
		h.errors.Abort(c, apiresponse.InvalidPayload(err))
		return
	}
	if !started {
		h.errors.Abort(c, &apiresponse.PublicError{Status: http.StatusConflict, Code: apiresponse.CodeConflict, Title: "Reports rebuild already active", Detail: "Only one reports rebuild may run at a time."})
		return
	}
	apiresponse.Success(c, http.StatusAccepted, gin.H{"status": "accepted"})
}

// Health godoc
// @Summary Get reports projection health (v1 admin)
// @Tags Reports v1
// @Produce json
// @Success 200 {object} apiresponse.SuccessResponse
// @Failure 401,403,503 {object} apiresponse.ProblemDetails
// @Router /api/v1/admin/reports/health [get]
func (h *ReportsV1AdminHandler) Health(c *gin.Context) {
	if h.rebuild == nil {
		h.errors.Abort(c, apiresponse.Unavailable(nil))
		return
	}
	health, err := h.rebuild.Health(c.Request.Context())
	if err != nil {
		h.errors.Abort(c, apiresponse.Unavailable(err))
		return
	}
	apiresponse.Success(c, http.StatusOK, health)
}

func (h *ReportsV1AdminHandler) query(c *gin.Context, withLimit bool) (dateRangeQuery, reportsDomain.DateRange, bool) {
	var query dateRangeQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		h.errors.Abort(c, apiresponse.InvalidPayload(err))
		return dateRangeQuery{}, reportsDomain.DateRange{}, false
	}
	if !withLimit {
		query.Limit = 0
	}
	from, err := time.Parse("2006-01-02", query.From)
	if err != nil {
		h.errors.Abort(c, apiresponse.InvalidPayload(err))
		return dateRangeQuery{}, reportsDomain.DateRange{}, false
	}
	to, err := time.Parse("2006-01-02", query.To)
	if err != nil {
		h.errors.Abort(c, apiresponse.InvalidPayload(err))
		return dateRangeQuery{}, reportsDomain.DateRange{}, false
	}
	return query, reportsDomain.DateRange{From: from, To: to}, true
}

func parseDateRange(fromValue, toValue string) (reportsDomain.DateRange, error) {
	from, err := time.Parse("2006-01-02", fromValue)
	if err != nil {
		return reportsDomain.DateRange{}, err
	}
	to, err := time.Parse("2006-01-02", toValue)
	if err != nil {
		return reportsDomain.DateRange{}, err
	}
	return reportsDomain.DateRange{From: from, To: to}, nil
}

// RegisterV1Routes attaches only protected read endpoints. No route is
// registered when Reports is disabled because the service is nil.
func RegisterV1Routes(group *gin.RouterGroup, authorizer adminDomain.Authorizer, service reportsDomain.QueryService, rebuild reportsDomain.Rebuilder, renderer *apiresponse.ErrorRenderer) {
	if group == nil || service == nil || renderer == nil {
		return
	}
	handler := NewReportsV1AdminHandler(service, rebuild, renderer)
	group.GET("/reports/revenue", shared.RequirePermissionV1(authorizer, PermissionReportsRead, renderer), handler.Revenue)
	group.GET("/reports/products/top", shared.RequirePermissionV1(authorizer, PermissionReportsRead, renderer), handler.TopProducts)
	group.GET("/reports/funnel", shared.RequirePermissionV1(authorizer, PermissionReportsRead, renderer), handler.Funnel)
	group.POST("/reports/rebuild", shared.RequirePermissionV1(authorizer, PermissionReportsRebuild, renderer), handler.Rebuild)
	group.GET("/reports/health", shared.RequirePermissionV1(authorizer, PermissionReportsRead, renderer), handler.Health)
}
