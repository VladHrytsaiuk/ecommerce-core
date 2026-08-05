package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	identityDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/identity/domain"
)

func TestLoginRouteReturnsJWT(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterRoutes(router.Group("/api"), fakeLogin{result: &identityDomain.LoginResult{AccessToken: "jwt", Role: "owner", ExpiresAt: time.Now()}}, nil)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"email":"owner@example.com","password":"password"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"access_token":"jwt"`) {
		t.Fatalf("login response = %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestLoginRouteRejectsInvalidCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterRoutes(router.Group("/api"), fakeLogin{err: identityDomain.ErrInvalidCredentials}, nil)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"email":"owner@example.com","password":"wrong"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("login status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}

type fakeLogin struct {
	result *identityDomain.LoginResult
	err    error
}

func (f fakeLogin) Login(context.Context, string, string) (*identityDomain.LoginResult, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.result == nil {
		return nil, errors.New("missing result")
	}
	return f.result, nil
}
