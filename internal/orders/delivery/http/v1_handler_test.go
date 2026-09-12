package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/apiresponse"
	ordersDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/orders/domain"
)

// This route returns a customer their own orders, so the property that matters
// most is which customer the query is run for.

func TestOrdersAreListedForTheAuthenticatedCustomerOnly(t *testing.T) {
	// The subject comes from the token, never from the request, or one
	// customer could read another's order history by asking for it.
	customerID := uuid.New()
	service := &orderServiceFake{}
	router := newOrdersRouter(service, customerID)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/orders?page=2&limit=5", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if service.customerID != customerID {
		t.Fatalf("listed orders for %s, want the authenticated customer %s", service.customerID, customerID)
	}
	if service.page != 2 || service.limit != 5 {
		t.Fatalf("pagination = page %d limit %d, want 2 and 5", service.page, service.limit)
	}
}

func TestAnAnonymousCallerGetsNoOrders(t *testing.T) {
	service := &orderServiceFake{}
	gin.SetMode(gin.TestMode)
	renderer := apiresponse.NewErrorRenderer(nil)
	router := gin.New()
	router.Use(renderer.Middleware())
	RegisterV1Routes(router.Group("/api/v1/orders"), service, renderer)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/orders", nil))

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
	if service.calls != 0 {
		t.Fatal("an anonymous request reached the order service")
	}
}

func TestPaginationBoundsAreEnforcedBeforeTheService(t *testing.T) {
	// An unbounded limit turns this into a full table read, and page numbers
	// beyond the cap into deep-offset scans.
	for name, query := range map[string]string{
		"page zero":         "?page=0",
		"page negative":     "?page=-1",
		"page too deep":     "?page=1001",
		"limit zero":        "?limit=0",
		"limit too large":   "?limit=101",
		"page not a number": "?page=abc",
	} {
		t.Run(name, func(t *testing.T) {
			service := &orderServiceFake{}
			recorder := httptest.NewRecorder()
			newOrdersRouter(service, uuid.New()).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/orders"+query, nil))

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
			}
			if service.calls != 0 {
				t.Fatal("an out-of-range page reached the order service")
			}
		})
	}
}

func TestTheResponseCarriesTheStandardPaginationEnvelope(t *testing.T) {
	service := &orderServiceFake{page: 1, result: ordersDomain.Page{
		Total:  3,
		Orders: []ordersDomain.Order{{ID: uuid.New(), Number: "ORD-1", Status: ordersDomain.StatusPaid, Total: mustMoney(t, 1500), CreatedAt: time.Now()}},
	}}

	recorder := httptest.NewRecorder()
	newOrdersRouter(service, uuid.New()).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/orders?limit=2", nil))

	var body struct {
		Data []struct {
			Number string `json:"number"`
			Status string `json:"status"`
		} `json:"data"`
		Meta struct {
			Total       int64 `json:"total"`
			TotalPages  int   `json:"total_pages"`
			HasNext     bool  `json:"has_next"`
			HasPrevious bool  `json:"has_previous"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not the shared envelope: %v — %s", err, recorder.Body.String())
	}
	if len(body.Data) != 1 || body.Data[0].Number != "ORD-1" {
		t.Fatalf("data = %+v", body.Data)
	}
	// Three orders at two per page is two pages, and the first has a next.
	if body.Meta.Total != 3 || body.Meta.TotalPages != 2 || !body.Meta.HasNext || body.Meta.HasPrevious {
		t.Fatalf("meta = %+v", body.Meta)
	}
}

func TestAnEmptyHistoryStillReportsOnePage(t *testing.T) {
	// Zero pages would make a client's "page 1 of 0" nonsensical.
	service := &orderServiceFake{result: ordersDomain.Page{}}

	recorder := httptest.NewRecorder()
	newOrdersRouter(service, uuid.New()).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/orders", nil))

	var body struct {
		Data []any `json:"data"`
		Meta struct {
			TotalPages int `json:"total_pages"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Meta.TotalPages != 1 {
		t.Fatalf("total_pages = %d, want 1", body.Meta.TotalPages)
	}
	// An empty list, not a null, so a client can iterate without a nil check.
	if body.Data == nil || len(body.Data) != 0 {
		t.Fatalf("data = %v, want an empty array", body.Data)
	}
}

func newOrdersRouter(service ordersDomain.Service, customerID uuid.UUID) *gin.Engine {
	gin.SetMode(gin.TestMode)
	renderer := apiresponse.NewErrorRenderer(nil)
	router := gin.New()
	router.Use(renderer.Middleware())
	group := router.Group("/api/v1/orders")
	group.Use(func(c *gin.Context) { c.Set("user_id", customerID) })
	RegisterV1Routes(group, service, renderer)
	return router
}

func mustMoney(t *testing.T, amount int64) money.Money {
	t.Helper()
	value, err := money.NewMoney(amount, "EUR")
	if err != nil {
		t.Fatal(err)
	}
	return value
}

type orderServiceFake struct {
	ordersDomain.Service
	calls      int
	customerID uuid.UUID
	page       int
	limit      int
	result     ordersDomain.Page
}

func (f *orderServiceFake) ListByCustomer(_ context.Context, customerID uuid.UUID, page, limit int) (ordersDomain.Page, error) {
	f.calls++
	f.customerID, f.page, f.limit = customerID, page, limit
	return f.result, nil
}
