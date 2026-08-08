package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware"
	identityDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/identity/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/security/token"
)

func TestSessionRoutesDisableCaching(t *testing.T) {
	gin.SetMode(gin.TestMode)
	userID := uuid.New()
	router := gin.New()
	RegisterRoutes(router.Group("/api"), fakeAuth{session: identityDomain.Session{AccessToken: "jwt", UserID: userID, Role: identityDomain.RoleOwner, ExpiresAt: time.Now()}}, nil, "https://store.example.test/api/auth/oauth/google/callback", nil, nil)

	for _, testCase := range []struct {
		name, method, target, body string
		wantStatus                 int
	}{
		{name: "register", method: http.MethodPost, target: "/api/auth/register", body: `{"email":"owner@example.com","password":"password"}`, wantStatus: http.StatusCreated},
		{name: "login", method: http.MethodPost, target: "/api/auth/login", body: `{"email":"owner@example.com","password":"password"}`, wantStatus: http.StatusOK},
		{name: "oauth callback", method: http.MethodGet, target: "/api/auth/oauth/google/callback?code=code&state=state", wantStatus: http.StatusOK},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(testCase.method, testCase.target, strings.NewReader(testCase.body))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(recorder, request)
			if recorder.Code != testCase.wantStatus || !strings.Contains(recorder.Body.String(), `"access_token":"jwt"`) {
				t.Fatalf("session response = %d %s", recorder.Code, recorder.Body.String())
			}
			if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
				t.Fatalf("Cache-Control = %q, want no-store", got)
			}
			if got := recorder.Header().Get("Pragma"); got != "no-cache" {
				t.Fatalf("Pragma = %q, want no-cache", got)
			}
		})
	}
}

func TestProfileRoutesRequireAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterRoutes(router.Group("/api"), nil, fakeProfile{}, "", func(c *gin.Context) {
		c.AbortWithStatus(http.StatusUnauthorized)
	}, nil)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/me/profile", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("profile status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}

func TestProfileUpdateReturnsConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	maker, err := token.NewJWTMaker("a-secure-secret-with-at-least-thirty-two-characters")
	if err != nil {
		t.Fatal(err)
	}
	accessToken, _, err := maker.CreateTokenForRole(uuid.New(), "customer", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	RegisterRoutes(router.Group("/api"), nil, fakeProfile{updateErr: identityDomain.ErrProfileConflict}, "", middleware.AuthMiddleware(maker), nil)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPatch, "/api/me/profile", strings.NewReader(`{"first_name":"Grace"}`))
	request.Header.Set("Authorization", "Bearer "+accessToken)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("PATCH status = %d, want %d: %s", recorder.Code, http.StatusConflict, recorder.Body.String())
	}
}

type fakeAuth struct {
	session identityDomain.Session
	err     error
}

func (f fakeAuth) RegisterPassword(context.Context, identityDomain.RegisterPasswordCommand) (identityDomain.Session, error) {
	return f.session, f.err
}

func (f fakeAuth) LoginPassword(context.Context, identityDomain.PasswordLoginCommand) (identityDomain.Session, error) {
	return f.session, f.err
}

func (f fakeAuth) BeginOAuth(context.Context, identityDomain.BeginOAuthCommand) (identityDomain.OAuthAuthorization, error) {
	return identityDomain.OAuthAuthorization{}, f.err
}

func (f fakeAuth) CompleteOAuth(context.Context, identityDomain.CompleteOAuthCommand) (identityDomain.Session, error) {
	return f.session, f.err
}

type fakeProfile struct{ updateErr error }

func (fakeProfile) GetProfile(context.Context, uuid.UUID) (*identityDomain.Profile, error) {
	return nil, identityDomain.ErrProfileNotFound
}

func (f fakeProfile) UpdateProfile(context.Context, uuid.UUID, []byte) (*identityDomain.Profile, error) {
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	return nil, identityDomain.ErrInvalidProfile
}
