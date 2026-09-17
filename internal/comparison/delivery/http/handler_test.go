package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/comparison/domain"
)

func TestGuestComparisonCreatesCartSessionCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &serviceFake{}
	router := gin.New()
	RegisterRoutes(router.Group("/api/comparison"), service, false)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/comparison", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if service.owner.SessionID == nil || *service.owner.SessionID == uuid.Nil {
		t.Fatal("guest comparison did not receive a generated session owner")
	}
	if len(response.Result().Cookies()) != 1 || response.Result().Cookies()[0].Name != "cart_session" {
		t.Fatalf("cookies = %+v, want one cart_session cookie", response.Result().Cookies())
	}
}

func TestComparisonLimitIsConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterRoutes(router.Group("/api/comparison"), &serviceFake{addErr: domain.ErrComparisonAtLimit}, false)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/comparison/"+uuid.NewString(), nil))
	if response.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusConflict)
	}
}

type serviceFake struct {
	owner  domain.Owner
	addErr error
}

func (service *serviceFake) List(_ context.Context, owner domain.Owner) (*domain.Comparison, error) {
	service.owner = owner
	return &domain.Comparison{Owner: owner, Lists: []domain.List{}}, nil
}
func (service *serviceFake) Add(context.Context, domain.Owner, uuid.UUID) error {
	return service.addErr
}
func (*serviceFake) Remove(context.Context, domain.Owner, uuid.UUID) error { return nil }
func (*serviceFake) MergeGuestComparison(context.Context, uuid.UUID, uuid.UUID) error {
	return nil
}
