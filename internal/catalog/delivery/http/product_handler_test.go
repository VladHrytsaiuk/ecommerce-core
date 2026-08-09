package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/domain"
)

func TestProductHandlerCreateAcceptsTranslationList(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeProductService{}
	handler := NewProductHandler(service)
	router := gin.New()
	router.POST("/products", handler.Create)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/products", strings.NewReader(`{
		"status":"draft",
		"translations":[
			{"locale":"es","name":"Crema","slug":"crema"},
			{"locale":"en","name":"Cream","slug":"cream"},
			{"locale":"ca","name":"Crema","slug":"crema-ca"}
		]
	}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if service.created == nil || len(service.created.Translations) != 3 {
		t.Fatalf("handler did not pass translation list to service: %+v", service.created)
	}
}

type fakeProductService struct{ created *domain.Product }

func (s *fakeProductService) Create(_ context.Context, product *domain.Product) error {
	s.created = product
	return nil
}

func (s *fakeProductService) FindBySlug(_ context.Context, _, _ string) (*domain.Product, error) {
	return nil, domain.ErrProductNotFound
}

func (s *fakeProductService) List(_ context.Context, _ string) ([]domain.Product, error) {
	return nil, nil
}
