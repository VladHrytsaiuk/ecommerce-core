package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/reviews/domain"
)

func TestCreateReviewRequiresJWT(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterRoutes(router.Group("/api"), router.Group("/api/admin"), &serviceFake{}, func(context *gin.Context) { context.AbortWithStatus(http.StatusUnauthorized) })

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/reviews/"+uuid.NewString(), strings.NewReader(`{"rating":5,"comment":"good"}`)))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func TestAdminStatusUpdateUsesValidatedService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterRoutes(router.Group("/api"), router.Group("/api/admin"), &serviceFake{}, nil)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPatch, "/api/admin/reviews/"+uuid.NewString()+"/status", strings.NewReader(`{"status":"invalid"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

type serviceFake struct{}

func (*serviceFake) Create(context.Context, domain.CreateCommand) (*domain.Review, error) {
	return nil, nil
}
func (*serviceFake) ListApproved(context.Context, uuid.UUID) ([]domain.Review, error) {
	return []domain.Review{}, nil
}
func (*serviceFake) SetStatus(context.Context, uuid.UUID, domain.Status) (*domain.Review, error) {
	return nil, domain.ErrInvalidReview
}
func (*serviceFake) Delete(context.Context, uuid.UUID) error { return nil }
