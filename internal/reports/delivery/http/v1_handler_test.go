package http

import (
	"bytes"
	"context"
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	adminDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/apiresponse"
	reportsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/reports/domain"
)

func TestReportsV1RoutesAuthorizeAndReturnRevenueEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	userID := uuid.New()
	renderer := apiresponse.NewErrorRenderer(nil)
	service := &queryFake{revenue: reportsDomain.RevenueSummary{Currency: "UAH", PaidOrdersCount: 2, NetRevenueMinor: 2_500}}
	router := gin.New()
	router.Use(renderer.Middleware(), func(c *gin.Context) { c.Set("user_id", userID); c.Next() })
	RegisterV1Routes(router.Group("/api/v1/admin"), authorizerFake{userID: userID}, service, rebuildFake{}, renderer)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodGet, "/api/v1/admin/reports/revenue?from=2026-01-01&to=2026-01-31&currency=UAH", nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Data      reportsDomain.RevenueSummary `json:"data"`
		RequestID string                       `json:"request_id"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Data.NetRevenueMinor != 2_500 || response.RequestID == "" {
		t.Fatalf("response = %+v", response)
	}
	if service.period.From.Format("2006-01-02") != "2026-01-01" || service.currency != "UAH" {
		t.Fatalf("query = %+v / %q", service.period, service.currency)
	}
}

func TestReportsV1RoutesRejectMissingPermission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	renderer := apiresponse.NewErrorRenderer(nil)
	router := gin.New()
	router.Use(renderer.Middleware(), func(c *gin.Context) { c.Set("user_id", uuid.New()); c.Next() })
	RegisterV1Routes(router.Group("/api/v1/admin"), authorizerFake{}, &queryFake{}, rebuildFake{}, renderer)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodGet, "/api/v1/admin/reports/revenue?from=2026-01-01&to=2026-01-31&currency=UAH", nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusForbidden {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestReportsV1RoutesReturnFunnelEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	userID := uuid.New()
	renderer := apiresponse.NewErrorRenderer(nil)
	service := &queryFake{}
	router := gin.New()
	router.Use(renderer.Middleware(), func(c *gin.Context) { c.Set("user_id", userID); c.Next() })
	RegisterV1Routes(router.Group("/api/v1/admin"), authorizerFake{userID: userID}, service, rebuildFake{}, renderer)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodGet, "/api/v1/admin/reports/funnel?from=2026-01-01&to=2026-01-31", nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if service.period.From.Format("2006-01-02") != "2026-01-01" || service.period.To.Format("2006-01-02") != "2026-01-31" {
		t.Fatalf("period = %+v", service.period)
	}
}

func TestReportsV1RoutesStartProtectedRebuild(t *testing.T) {
	gin.SetMode(gin.TestMode)
	userID := uuid.New()
	renderer := apiresponse.NewErrorRenderer(nil)
	router := gin.New()
	router.Use(renderer.Middleware(), func(c *gin.Context) { c.Set("user_id", userID); c.Next() })
	RegisterV1Routes(router.Group("/api/v1/admin"), authorizerFake{userID: userID}, &queryFake{}, rebuildFake{}, renderer)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodPost, "/api/v1/admin/reports/rebuild", bytes.NewBufferString(`{"from":"2026-01-01","to":"2026-01-31"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusAccepted {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestReportsV1RoutesReturnProjectionHealth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	userID := uuid.New()
	renderer := apiresponse.NewErrorRenderer(nil)
	router := gin.New()
	router.Use(renderer.Middleware(), func(c *gin.Context) { c.Set("user_id", userID); c.Next() })
	RegisterV1Routes(router.Group("/api/v1/admin"), authorizerFake{userID: userID}, &queryFake{}, rebuildFake{}, renderer)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodGet, "/api/v1/admin/reports/health", nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

type authorizerFake struct{ userID uuid.UUID }

func (f authorizerFake) Require(_ context.Context, userID uuid.UUID, permission string) error {
	if userID == f.userID && (permission == PermissionReportsRead || permission == PermissionReportsRebuild) {
		return nil
	}
	return adminDomain.ErrPermissionDenied
}

type queryFake struct {
	revenue  reportsDomain.RevenueSummary
	products []reportsDomain.TopProductSales
	period   reportsDomain.DateRange
	currency string
	limit    int
}

type rebuildFake struct{}

func (rebuildFake) StartAsync(reportsDomain.DateRange) (bool, error) { return true, nil }
func (rebuildFake) Health(context.Context) (reportsDomain.Health, error) {
	return reportsDomain.Health{}, nil
}

func (f *queryFake) Revenue(_ context.Context, period reportsDomain.DateRange, currency string) (reportsDomain.RevenueSummary, error) {
	f.period, f.currency = period, currency
	return f.revenue, nil
}
func (f *queryFake) TopProducts(_ context.Context, period reportsDomain.DateRange, currency string, limit int) ([]reportsDomain.TopProductSales, error) {
	f.period, f.currency, f.limit = period, currency, limit
	return f.products, nil
}
func (f *queryFake) Funnel(_ context.Context, period reportsDomain.DateRange) ([]reportsDomain.DailyFunnel, error) {
	f.period = period
	return []reportsDomain.DailyFunnel{{CartsCreated: 1}}, nil
}

var _ reportsDomain.QueryService = (*queryFake)(nil)
var _ adminDomain.Authorizer = authorizerFake{}
