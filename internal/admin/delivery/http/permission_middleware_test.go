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
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/apiresponse"
	sharedMiddleware "github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/security/token"
)

// The subject being authorized is the one AuthMiddleware established, never
// anything the caller supplied in the request.
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
	renderer := apiresponse.NewErrorRenderer(nil)
	router.Use(renderer.Middleware())
	router.POST("/admin/catalog", sharedMiddleware.AuthMiddleware(maker), RequirePermissionV1(authorizer, "catalog:write", renderer), func(c *gin.Context) { c.Status(http.StatusNoContent) })

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/admin/catalog", nil)
	request.Header.Set("Authorization", "Bearer "+jwt)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}

	// A different subject holding no such permission is refused, so the pass
	// above cannot come from the middleware ignoring the subject entirely.
	other, _, err := maker.CreateTokenForRole(uuid.New(), token.RoleCustomer, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/admin/catalog", nil)
	request.Header.Set("Authorization", "Bearer "+other)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status for an unauthorized subject = %d, want %d", recorder.Code, http.StatusForbidden)
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
