package http

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identityDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/identity/domain"
)

// emailCodeAuth records the commands the handlers pass on.
type emailCodeAuth struct {
	fakeAuth
	requested []identityDomain.RequestSignInCodeCommand
	verified  []identityDomain.VerifySignInCodeCommand
}

func (f *emailCodeAuth) RequestSignInCode(_ context.Context, command identityDomain.RequestSignInCodeCommand) (identityDomain.CodeRequest, error) {
	f.requested = append(f.requested, command)
	return f.codeRequest, f.err
}

func (f *emailCodeAuth) VerifySignInCode(_ context.Context, command identityDomain.VerifySignInCodeCommand) (identityDomain.Session, error) {
	f.verified = append(f.verified, command)
	return f.session, f.err
}

func newEmailCodeRouter(service identityDomain.AuthService, loginLimit gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterRoutes(router.Group("/api"), service, nil, SignInMethods{EmailCode: true}, "", nil, nil, loginLimit)
	return router
}

func TestRequestingAnEmailCodeAnswersWhenItStopsWorking(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	service := &emailCodeAuth{fakeAuth: fakeAuth{codeRequest: identityDomain.CodeRequest{ExpiresAt: now.Add(10 * time.Minute), ResendAfter: now.Add(time.Minute)}}}
	router := newEmailCodeRouter(service, nil)

	recorder := postJSON(router, "/api/auth/email-code", `{"email":"buyer@example.com"}`)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d body = %s, want 202", recorder.Code, recorder.Body.String())
	}
	if body := recorder.Body.String(); body != `{"expires_at":"2026-09-17T12:10:00Z","resend_after":"2026-09-17T12:01:00Z"}` {
		t.Fatalf("body = %s", body)
	}
	if len(service.requested) != 1 || service.requested[0] != (identityDomain.RequestSignInCodeCommand{Channel: identityDomain.CodeChannelEmail, Destination: "buyer@example.com"}) {
		t.Fatalf("requested = %+v, want the address on the email channel", service.requested)
	}
}

func TestAVerifiedEmailCodeReturnsASessionThatIsNeverCached(t *testing.T) {
	service := &emailCodeAuth{fakeAuth: fakeAuth{session: identityDomain.Session{AccessToken: "access", UserID: uuid.New(), Role: identityDomain.RoleCustomer, RefreshToken: "refresh"}}}
	router := newEmailCodeRouter(service, nil)

	recorder := postJSON(router, "/api/auth/email-code/verify", `{"email":"buyer@example.com","code":"123456"}`)

	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"refresh_token":"refresh"`) {
		t.Fatalf("status = %d body = %s, want the session", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("a response carrying a session must not be cached")
	}
	want := identityDomain.VerifySignInCodeCommand{Channel: identityDomain.CodeChannelEmail, Destination: "buyer@example.com", Code: "123456"}
	if len(service.verified) != 1 || service.verified[0] != want {
		t.Fatalf("verified = %+v, want %+v", service.verified, want)
	}
}

func TestEmailCodeRefusalsMapToTheirStatus(t *testing.T) {
	for name, testCase := range map[string]struct {
		path, body string
		err        error
		status     int
		retryAfter string
	}{
		"a throttled address":   {"/api/auth/email-code", `{"email":"buyer@example.com"}`, &identityDomain.CodeThrottledError{RetryAfter: 1500 * time.Millisecond}, http.StatusTooManyRequests, "2"},
		"not an address":        {"/api/auth/email-code", `{"email":"buyer"}`, identityDomain.ErrInvalidCodeDestination, http.StatusBadRequest, ""},
		"a wrong code":          {"/api/auth/email-code/verify", `{"email":"buyer@example.com","code":"000000"}`, identityDomain.ErrInvalidCode, http.StatusUnauthorized, ""},
		"a disabled account":    {"/api/auth/email-code/verify", `{"email":"buyer@example.com","code":"123456"}`, identityDomain.ErrInvalidCredentials, http.StatusUnauthorized, ""},
		"a missing code":        {"/api/auth/email-code/verify", `{"email":"buyer@example.com"}`, nil, http.StatusBadRequest, ""},
		"a missing address":     {"/api/auth/email-code", `{}`, nil, http.StatusBadRequest, ""},
		"a method switched off": {"/api/auth/email-code", `{"email":"buyer@example.com"}`, identityDomain.ErrSignInMethodDisabled, http.StatusNotFound, ""},
	} {
		t.Run(name, func(t *testing.T) {
			router := newEmailCodeRouter(&emailCodeAuth{fakeAuth: fakeAuth{err: testCase.err}}, nil)
			recorder := postJSON(router, testCase.path, testCase.body)
			if recorder.Code != testCase.status {
				t.Fatalf("status = %d body = %s, want %d", recorder.Code, recorder.Body.String(), testCase.status)
			}
			if got := recorder.Header().Get("Retry-After"); got != testCase.retryAfter {
				t.Fatalf("Retry-After = %q, want %q", got, testCase.retryAfter)
			}
		})
	}
}

func TestEmailCodeRoutesShareTheLoginLimit(t *testing.T) {
	limited := 0
	limit := func(c *gin.Context) {
		limited++
		c.AbortWithStatus(http.StatusTooManyRequests)
	}
	service := &emailCodeAuth{}
	router := newEmailCodeRouter(service, limit)

	for _, request := range []struct{ path, body string }{
		{"/api/auth/email-code", `{"email":"buyer@example.com"}`},
		{"/api/auth/email-code/verify", `{"email":"buyer@example.com","code":"123456"}`},
	} {
		if recorder := postJSON(router, request.path, request.body); recorder.Code != http.StatusTooManyRequests {
			t.Fatalf("%s status = %d, want the limit applied", request.path, recorder.Code)
		}
	}
	if limited != 2 || len(service.requested)+len(service.verified) != 0 {
		t.Fatalf("limited %d times, service reached %d times; want both routes stopped by the limit", limited, len(service.requested)+len(service.verified))
	}
}
