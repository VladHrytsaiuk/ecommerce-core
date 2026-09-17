package http

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identityDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/identity/domain"
)

func newAuthRouter(service identityDomain.AuthService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterRoutes(router.Group("/api"), service, nil, SignInMethods{Password: true, OAuth: true}, "", nil, nil)
	return router
}

func postJSON(router *gin.Engine, path, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestARefreshReturnsANewSessionAndIsNeverCached(t *testing.T) {
	expires := time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC)
	router := newAuthRouter(fakeAuth{session: identityDomain.Session{
		AccessToken: "access", ExpiresAt: expires.Add(-7 * 24 * time.Hour), UserID: uuid.New(), Role: identityDomain.RoleCustomer,
		RefreshToken: "next-refresh-token", RefreshExpiresAt: expires,
	}})

	recorder := postJSON(router, "/api/auth/refresh", `{"refresh_token":"presented"}`)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	for _, expected := range []string{`"refresh_token":"next-refresh-token"`, `"refresh_expires_at":"2026-09-22T10:00:00Z"`} {
		if !strings.Contains(recorder.Body.String(), expected) {
			t.Fatalf("body = %s, want %s", recorder.Body.String(), expected)
		}
	}
	if recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q, a response carrying a refresh token must not be cached", recorder.Header().Get("Cache-Control"))
	}
}

func TestARefusedRefreshIsUnauthorized(t *testing.T) {
	router := newAuthRouter(fakeAuth{err: identityDomain.ErrInvalidRefreshToken})
	if recorder := postJSON(router, "/api/auth/refresh", `{"refresh_token":"stolen"}`); recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body = %s, want 401", recorder.Code, recorder.Body.String())
	}
}

func TestASessionWithoutARefreshTokenOmitsTheFields(t *testing.T) {
	// A deployment without refresh tokens must not advertise an empty one.
	router := newAuthRouter(fakeAuth{session: identityDomain.Session{AccessToken: "access", UserID: uuid.New(), Role: identityDomain.RoleCustomer}})
	recorder := postJSON(router, "/api/auth/refresh", `{"refresh_token":"presented"}`)
	if strings.Contains(recorder.Body.String(), "refresh_token") || strings.Contains(recorder.Body.String(), "refresh_expires_at") {
		t.Fatalf("body = %s, want no refresh fields", recorder.Body.String())
	}
}

func TestLogoutAnswersNoContentAndRevealsNothing(t *testing.T) {
	router := newAuthRouter(fakeAuth{})
	if recorder := postJSON(router, "/api/auth/logout", `{"refresh_token":"any"}`); recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d body = %s, want 204", recorder.Code, recorder.Body.String())
	}
}

func TestRefreshAndLogoutRequireAToken(t *testing.T) {
	router := newAuthRouter(fakeAuth{})
	for _, path := range []string{"/api/auth/refresh", "/api/auth/logout"} {
		if recorder := postJSON(router, path, `{}`); recorder.Code != http.StatusBadRequest {
			t.Fatalf("%s status = %d, want 400", path, recorder.Code)
		}
	}
}

func TestALogoutThatCannotBeRecordedIsNotReportedAsDone(t *testing.T) {
	router := newAuthRouter(fakeAuth{err: errors.New("database unavailable")})
	if recorder := postJSON(router, "/api/auth/logout", `{"refresh_token":"any"}`); recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 rather than a 204 for a revocation that did not happen", recorder.Code)
	}
}
