package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identityDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/identity/domain"
)

func routeStatus(router *gin.Engine, method, path string) int {
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(method, path, nil))
	return recorder.Code
}

func TestADisabledSignInMethodHasNoRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := fakeAuth{session: identityDomain.Session{AccessToken: "access", UserID: uuid.New(), Role: identityDomain.RoleCustomer}}
	for name, testCase := range map[string]struct {
		methods        SignInMethods
		passwordRouted bool
		oauthRouted    bool
	}{
		"password only": {SignInMethods{Password: true}, true, false},
		"google only":   {SignInMethods{OAuth: true}, false, true},
		"both":          {SignInMethods{Password: true, OAuth: true}, true, true},
	} {
		t.Run(name, func(t *testing.T) {
			router := gin.New()
			RegisterRoutes(router.Group("/api"), service, nil, testCase.methods, "https://store.example.test/callback", nil, nil)

			for _, path := range []string{"/api/auth/login", "/api/auth/register"} {
				if routed := routeStatus(router, http.MethodPost, path) != http.StatusNotFound; routed != testCase.passwordRouted {
					t.Fatalf("%s routed = %v, want %v", path, routed, testCase.passwordRouted)
				}
			}
			if routed := routeStatus(router, http.MethodGet, "/api/auth/oauth/google/login") != http.StatusNotFound; routed != testCase.oauthRouted {
				t.Fatalf("OAuth routed = %v, want %v", routed, testCase.oauthRouted)
			}
			// Every method ends in a session, so refresh is always there.
			if routeStatus(router, http.MethodPost, "/api/auth/refresh") == http.StatusNotFound {
				t.Fatal("refresh is missing")
			}
		})
	}
}
