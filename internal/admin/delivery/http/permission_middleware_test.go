package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	adminDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/domain"
	sharedMiddleware "github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/security/token"
)

func TestRequirePermissionUsesAuthenticatedSubject(t *testing.T) {
	gin.SetMode(gin.TestMode)
	userID := uuid.New()
	maker, err := token.NewJWTMaker("01234567890123456789012345678901")
	if err != nil {
		t.Fatal(err)
	}
	jwt, _, err := maker.CreateTokenForRole(userID, token.RoleCustomer, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	authorizer := authorizerFake{allowedUserID: userID, allowedPermission: "catalog:write"}
	router := gin.New()
	router.POST("/admin/catalog", sharedMiddleware.AuthMiddleware(maker), RequirePermission(authorizer, "catalog:write"), func(c *gin.Context) { c.Status(http.StatusNoContent) })

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/admin/catalog", nil)
	request.Header.Set("Authorization", "Bearer "+jwt)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
}

type authorizerFake struct {
	allowedUserID     uuid.UUID
	allowedPermission string
}

func (f authorizerFake) Require(_ context.Context, userID uuid.UUID, permission string) error {
	if userID == f.allowedUserID && permission == f.allowedPermission {
		return nil
	}
	return adminDomain.ErrPermissionDenied
}

var _ adminDomain.Authorizer = authorizerFake{}
