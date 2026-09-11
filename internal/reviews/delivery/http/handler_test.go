package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/apiresponse"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/reviews/domain"
)

func TestSubmittingAReviewRequiresAuthentication(t *testing.T) {
	router, _ := newReviewRouter(t, &serviceFake{})

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost,
		"/api/v1/catalog/products/"+uuid.NewString()+"/reviews", strings.NewReader(`{"rating":5,"comment":"good"}`)))

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func TestApprovedReviewsAreReadableWithoutAuthentication(t *testing.T) {
	// The read side is the whole point of the module for a storefront; it must
	// not sit behind the same gate as writing one.
	approved := domain.Review{ID: uuid.New(), ProductID: uuid.New(), Rating: 5, Comment: "good", Status: domain.StatusApproved}
	router, _ := newReviewRouter(t, &serviceFake{approved: []domain.Review{approved}})

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet,
		"/api/v1/catalog/products/"+approved.ProductID.String()+"/reviews", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var body struct {
		Data struct {
			Reviews []reviewResponse `json:"reviews"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data.Reviews) != 1 || body.Data.Reviews[0].Status != domain.StatusApproved {
		t.Fatalf("reviews = %+v", body.Data.Reviews)
	}
	// RFC 3339 rather than the HTTP date format the legacy handler emitted;
	// every other v1 timestamp in this API is RFC 3339.
	if _, err := time.Parse(time.RFC3339, body.Data.Reviews[0].CreatedAt); err != nil {
		t.Fatalf("created_at = %q, want RFC 3339", body.Data.Reviews[0].CreatedAt)
	}
}

func TestASecondReviewOfTheSameProductConflicts(t *testing.T) {
	router, _ := newReviewRouter(t, &serviceFake{createErr: domain.ErrAlreadyExists})

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost,
		"/api/v1/catalog/products/"+uuid.NewString()+"/reviews", strings.NewReader(`{"rating":5,"comment":"good"}`))
	request.Header.Set("Content-Type", "application/json")
	authenticate(request)
	router.ServeHTTP(response, request)

	if response.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d, body = %s", response.Code, http.StatusConflict, response.Body.String())
	}
}

func TestAMalformedProductIDIsRejectedBeforeTheService(t *testing.T) {
	service := &serviceFake{}
	router, _ := newReviewRouter(t, service)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/catalog/products/not-a-uuid/reviews", nil))

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	if service.listed != 0 {
		t.Fatal("the service was called with an unparsed product id")
	}
}

func TestModerationIsNotExposedHere(t *testing.T) {
	// These routes existed with no authorization and no audit record. They
	// belong to ContentAdminFacade, which writes the change and its audit
	// entry in one transaction; re-adding them here would be the bypass.
	router, _ := newReviewRouter(t, &serviceFake{})
	reviewID := uuid.NewString()

	for _, route := range []struct{ method, path string }{
		{http.MethodPatch, "/api/v1/catalog/products/" + reviewID + "/reviews/status"},
		{http.MethodDelete, "/api/v1/catalog/products/" + reviewID + "/reviews"},
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(route.method, route.path, nil))
		if response.Code != http.StatusNotFound && response.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s %s = %d, want the route to be absent", route.method, route.path, response.Code)
		}
	}
}

// authenticate stands in for the auth middleware having accepted a token.
const testUserHeader = "X-Test-User"

func authenticate(request *http.Request) { request.Header.Set(testUserHeader, uuid.NewString()) }

func newReviewRouter(t *testing.T, service domain.Service) (*gin.Engine, *serviceFake) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	renderer := apiresponse.NewErrorRenderer(nil)
	router.Use(renderer.Middleware())
	v1 := router.Group("/api/v1")
	RegisterV1Routes(v1, service, func(context *gin.Context) {
		id, err := uuid.Parse(context.GetHeader(testUserHeader))
		if err != nil {
			renderer.Abort(context, apiresponse.Unauthenticated(nil))
			context.Abort()
			return
		}
		context.Set("user_id", id)
	}, renderer)
	fake, _ := service.(*serviceFake)
	return router, fake
}

type serviceFake struct {
	approved  []domain.Review
	createErr error
	listed    int
}

func (s *serviceFake) Create(context.Context, domain.CreateCommand) (*domain.Review, error) {
	if s.createErr != nil {
		return nil, s.createErr
	}
	return &domain.Review{ID: uuid.New(), Status: domain.StatusPending}, nil
}
func (s *serviceFake) ListApproved(context.Context, uuid.UUID) ([]domain.Review, error) {
	s.listed++
	return s.approved, nil
}
func (s *serviceFake) SetStatus(context.Context, uuid.UUID, domain.Status) (*domain.Review, error) {
	return nil, domain.ErrInvalidReview
}
func (s *serviceFake) Delete(context.Context, uuid.UUID) error { return nil }
