package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identityDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/identity/domain"
)

// accountCodesAuth records what the verification and reset handlers pass on.
type accountCodesAuth struct {
	fakeAuth
	verificationFor []uuid.UUID
	confirmed       []identityDomain.ConfirmEmailCommand
	resets          []identityDomain.ResetPasswordCommand
}

func (f *accountCodesAuth) RequestEmailVerification(_ context.Context, userID uuid.UUID) (identityDomain.CodeRequest, error) {
	f.verificationFor = append(f.verificationFor, userID)
	return f.codeRequest, f.err
}

func (f *accountCodesAuth) ConfirmEmail(_ context.Context, command identityDomain.ConfirmEmailCommand) error {
	f.confirmed = append(f.confirmed, command)
	return f.err
}

func (f *accountCodesAuth) ResetPassword(_ context.Context, command identityDomain.ResetPasswordCommand) (identityDomain.Session, error) {
	f.resets = append(f.resets, command)
	return f.session, f.err
}

// signedInAs stands in for the auth middleware, which sets the user id.
func signedInAs(userID uuid.UUID) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetHeader("Authorization") == "" {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		// The key and type middleware.AuthMiddleware sets.
		c.Set("user_id", userID)
		c.Next()
	}
}

func newAccountCodesRouter(service identityDomain.AuthService, methods SignInMethods, userID uuid.UUID) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterRoutes(router.Group("/api"), service, nil, methods, "", signedInAs(userID), RouteLimits{})
	return router
}

func postAuthorized(router *gin.Engine, path, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer token")
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestVerificationAndResetExistOnlyForPasswordAccountsWithCodes(t *testing.T) {
	paths := []string{"/api/auth/password-reset", "/api/auth/password-reset/confirm", "/api/auth/email-verification", "/api/auth/email-verification/confirm"}
	for name, testCase := range map[string]struct {
		methods SignInMethods
		routed  bool
	}{
		"password with codes":    {SignInMethods{Password: true, PasswordCodes: true}, true},
		"password without codes": {SignInMethods{Password: true}, false},
		"codes without password": {SignInMethods{EmailCode: true, PasswordCodes: true}, false},
	} {
		t.Run(name, func(t *testing.T) {
			router := newAccountCodesRouter(&accountCodesAuth{}, testCase.methods, uuid.New())
			for _, path := range paths {
				if routed := postAuthorized(router, path, `{}`).Code != http.StatusNotFound; routed != testCase.routed {
					t.Fatalf("%s routed = %v, want %v", path, routed, testCase.routed)
				}
			}
		})
	}
}

func TestEmailVerificationActsForTheSignedInAccountOnly(t *testing.T) {
	userID := uuid.New()
	service := &accountCodesAuth{}
	router := newAccountCodesRouter(service, SignInMethods{Password: true, PasswordCodes: true}, userID)

	unauthenticated := postJSON(router, "/api/auth/email-verification", `{}`)
	if unauthenticated.Code != http.StatusUnauthorized || len(service.verificationFor) != 0 {
		t.Fatalf("status = %d, want 401 before the service is reached", unauthenticated.Code)
	}
	if recorder := postAuthorized(router, "/api/auth/email-verification", ``); recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d body = %s, want 202", recorder.Code, recorder.Body.String())
	}
	if recorder := postAuthorized(router, "/api/auth/email-verification/confirm", `{"code":"123456"}`); recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d body = %s, want 204", recorder.Code, recorder.Body.String())
	}
	if len(service.verificationFor) != 1 || service.verificationFor[0] != userID || len(service.confirmed) != 1 || service.confirmed[0] != (identityDomain.ConfirmEmailCommand{UserID: userID, Code: "123456"}) {
		t.Fatalf("service saw %v and %+v, want the signed-in account's id", service.verificationFor, service.confirmed)
	}
}

func TestAccountCodeRefusalsMapToTheirStatus(t *testing.T) {
	for name, testCase := range map[string]struct {
		path, body string
		err        error
		status     int
	}{
		// A signed-in caller given 401 would take it for a lost session.
		"a wrong confirmation code":   {"/api/auth/email-verification/confirm", `{"code":"000000"}`, identityDomain.ErrInvalidCode, http.StatusUnprocessableEntity},
		"an address already verified": {"/api/auth/email-verification", ``, identityDomain.ErrEmailAlreadyVerified, http.StatusConflict},
		"an account with no address":  {"/api/auth/email-verification", ``, identityDomain.ErrNoEmailToVerify, http.StatusBadRequest},
		"a wrong reset code":          {"/api/auth/password-reset/confirm", `{"email":"buyer@example.com","code":"000000","password":"new-password-123"}`, identityDomain.ErrInvalidCode, http.StatusUnauthorized},
		"a short new password":        {"/api/auth/password-reset/confirm", `{"email":"buyer@example.com","code":"123456","password":"short"}`, identityDomain.ErrInvalidPassword, http.StatusBadRequest},
		"a reset for a non-address":   {"/api/auth/password-reset", `{"email":"buyer"}`, identityDomain.ErrInvalidCodeDestination, http.StatusBadRequest},
		"a reset without a password":  {"/api/auth/password-reset/confirm", `{"email":"buyer@example.com","code":"123456"}`, nil, http.StatusBadRequest},
	} {
		t.Run(name, func(t *testing.T) {
			router := newAccountCodesRouter(&accountCodesAuth{fakeAuth: fakeAuth{err: testCase.err}}, SignInMethods{Password: true, PasswordCodes: true}, uuid.New())
			if recorder := postAuthorized(router, testCase.path, testCase.body); recorder.Code != testCase.status {
				t.Fatalf("status = %d body = %s, want %d", recorder.Code, recorder.Body.String(), testCase.status)
			}
		})
	}
}

func TestAPasswordResetSignsInWithAnUncachedSession(t *testing.T) {
	service := &accountCodesAuth{fakeAuth: fakeAuth{session: identityDomain.Session{AccessToken: "access", UserID: uuid.New(), Role: identityDomain.RoleCustomer}}}
	router := newAccountCodesRouter(service, SignInMethods{Password: true, PasswordCodes: true}, uuid.New())

	recorder := postJSON(router, "/api/auth/password-reset/confirm", `{"email":"buyer@example.com","code":"123456","password":"new-password-123"}`)
	if recorder.Code != http.StatusOK || recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("status = %d, Cache-Control = %q; want an uncached session", recorder.Code, recorder.Header().Get("Cache-Control"))
	}
	if len(service.resets) != 1 || service.resets[0].Password != "new-password-123" || service.resets[0].Code != "123456" {
		t.Fatalf("resets = %+v", service.resets)
	}
}
