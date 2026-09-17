package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/wishlist/domain"
)

func TestGuestWishlistCreatesOnlyServerIssuedSessionCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeService{}
	router := gin.New()
	RegisterRoutes(router.Group("/api/wishlist"), service, false)

	request := httptest.NewRequest(http.MethodGet, "/api/wishlist", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if len(response.Result().Cookies()) != 1 || response.Result().Cookies()[0].Name != "cart_session" {
		t.Fatalf("cookies = %+v, want one cart_session cookie", response.Result().Cookies())
	}
	if service.listOwner.SessionID == nil || *service.listOwner.SessionID == uuid.Nil {
		t.Fatal("guest list did not receive a generated session owner")
	}
}

func TestWishlistRejectsInvalidVariantID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeService{}
	router := gin.New()
	RegisterRoutes(router.Group("/api/wishlist"), service, false)

	request := httptest.NewRequest(http.MethodPost, "/api/wishlist/not-a-uuid", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	if service.addCalls != 0 {
		t.Fatalf("Add calls = %d, want 0", service.addCalls)
	}
}

type fakeService struct {
	listOwner domain.Owner
	addCalls  int
}

func (service *fakeService) List(_ context.Context, owner domain.Owner) (*domain.Wishlist, error) {
	service.listOwner = owner
	return &domain.Wishlist{Owner: owner, Items: []domain.WishlistItem{}}, nil
}

func (service *fakeService) Add(context.Context, domain.Owner, uuid.UUID) error {
	service.addCalls++
	return nil
}

func (service *fakeService) Remove(context.Context, domain.Owner, uuid.UUID) error { return nil }

func (service *fakeService) MergeGuestWishlist(context.Context, uuid.UUID, uuid.UUID) error {
	return nil
}
