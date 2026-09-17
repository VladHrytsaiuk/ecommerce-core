package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/cart/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/cartowner"
)

// The cart is the only thing a buyer writes before paying, and this package
// had no tests at all. These cover who the write is attributed to, what a
// malformed one is answered with, and that the session a guest is issued is
// the one their next request comes back with.

func TestAGuestIsIssuedASessionCookieAndKeepsIt(t *testing.T) {
	service := &cartServiceFake{}
	router := newCartRouter(service, true)

	first := httptest.NewRecorder()
	router.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/api/uk/cart", nil))
	if first.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", first.Code, first.Body.String())
	}
	cookie := sessionCookie(t, first.Result().Cookies())
	if cookie == nil {
		t.Fatal("no cart session cookie was issued to a guest")
	}
	if !cookie.HttpOnly || !cookie.Secure {
		t.Fatalf("cookie HttpOnly=%t Secure=%t; a cart session must not be readable by scripts or sent in the clear", cookie.HttpOnly, cookie.Secure)
	}
	issued := service.lastOwner.SessionID
	if issued == nil || cookie.Value != issued.String() {
		t.Fatalf("cookie %q does not name the session the cart was created for (%v)", cookie.Value, issued)
	}

	// The second request carries the cookie and must reuse that session
	// rather than starting a new cart.
	second := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/uk/cart", nil)
	request.AddCookie(cookie)
	router.ServeHTTP(second, request)
	if second.Code != http.StatusOK {
		t.Fatalf("status = %d", second.Code)
	}
	if service.lastOwner.SessionID == nil || service.lastOwner.SessionID.String() != cookie.Value {
		t.Fatalf("second request used session %v, want %s", service.lastOwner.SessionID, cookie.Value)
	}
	if len(second.Result().Cookies()) != 0 {
		t.Fatal("a returning guest was issued a second session cookie")
	}
}

func TestAnAuthenticatedBuyerWritesToTheirOwnCartNotAGuestOne(t *testing.T) {
	// Falling through to the cookie branch for a signed-in customer would put
	// their items in a guest cart that the next sign-in cannot find.
	customerID := uuid.New()
	service := &cartServiceFake{}
	// The middleware has to be in place before the routes are registered;
	// Gin builds each route's chain at registration time.
	router := newCartRouter(service, false, func(c *gin.Context) { c.Set("user_id", customerID) })

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/uk/cart/items",
		strings.NewReader(`{"variant_id":"`+uuid.NewString()+`","quantity":2}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if service.lastOwner.CustomerID == nil || *service.lastOwner.CustomerID != customerID {
		t.Fatalf("write attributed to %+v, want customer %s", service.lastOwner, customerID)
	}
	if service.lastOwner.SessionID != nil {
		t.Fatal("an authenticated buyer was given a guest session")
	}
	if len(recorder.Result().Cookies()) != 0 {
		t.Fatal("an authenticated buyer was issued a cart session cookie")
	}
}

func TestAMalformedCartWriteIsRejectedBeforeTheService(t *testing.T) {
	for name, testCase := range map[string]struct {
		method, path, body string
	}{
		"no variant":                  {http.MethodPost, "/api/uk/cart/items", `{"quantity":1}`},
		"zero quantity":               {http.MethodPost, "/api/uk/cart/items", `{"variant_id":"` + uuid.NewString() + `","quantity":0}`},
		"negative quantity":           {http.MethodPost, "/api/uk/cart/items", `{"variant_id":"` + uuid.NewString() + `","quantity":-3}`},
		"not json":                    {http.MethodPost, "/api/uk/cart/items", `nonsense`},
		"unparsable variant in path":  {http.MethodDelete, "/api/uk/cart/items/not-a-uuid", ""},
		"unparsable variant on patch": {http.MethodPatch, "/api/uk/cart/items/not-a-uuid", `{"variant_id":"` + uuid.NewString() + `","quantity":1}`},
	} {
		t.Run(name, func(t *testing.T) {
			service := &cartServiceFake{}
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(testCase.method, testCase.path, strings.NewReader(testCase.body))
			request.Header.Set("Content-Type", "application/json")
			newCartRouter(service, false).ServeHTTP(recorder, request)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
			}
			if service.calls != 0 {
				t.Fatal("a malformed request reached the cart service")
			}
		})
	}
}

func TestAPatchOverridesTheVariantInTheBodyWithTheOneInThePath(t *testing.T) {
	// The path names the line being changed. Trusting the body would let a
	// caller edit a different line than the URL says.
	pathVariant, bodyVariant := uuid.New(), uuid.New()
	service := &cartServiceFake{}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPatch, "/api/uk/cart/items/"+pathVariant.String(),
		strings.NewReader(`{"variant_id":"`+bodyVariant.String()+`","quantity":4}`))
	request.Header.Set("Content-Type", "application/json")
	newCartRouter(service, false).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if service.lastItem.VariantID != pathVariant {
		t.Fatalf("changed variant %s, want the one named in the path %s", service.lastItem.VariantID, pathVariant)
	}
	if service.lastItem.Quantity != 4 {
		t.Fatalf("quantity = %d, want 4", service.lastItem.Quantity)
	}
}

func TestAnUnknownLineIsNotFoundRatherThanUnprocessable(t *testing.T) {
	service := &cartServiceFake{err: domain.ErrItemNotFound}

	recorder := httptest.NewRecorder()
	newCartRouter(service, false).ServeHTTP(recorder,
		httptest.NewRequest(http.MethodDelete, "/api/uk/cart/items/"+uuid.NewString(), nil))

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}

func TestACorruptSessionCookieIsRefusedRatherThanReplaced(t *testing.T) {
	// Silently issuing a new session would detach the buyer from the cart they
	// were building, with no error to explain where it went.
	service := &cartServiceFake{}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/uk/cart", nil)
	request.AddCookie(&http.Cookie{Name: cartowner.SessionCookie, Value: "not-a-uuid"})
	newCartRouter(service, false).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	if service.calls != 0 {
		t.Fatal("a corrupt session reached the cart service")
	}
}

func TestAnUnexpectedFailureIsNotDescribedToTheBuyer(t *testing.T) {
	// A database failure used to come back as 422 carrying the driver's text,
	// which names tables, columns and constraints — and 422 tells a client the
	// request was malformed and not worth retrying.
	service := &cartServiceFake{err: errors.New("pq: relation \"cart_items\" does not exist")}

	recorder := httptest.NewRecorder()
	newCartRouter(service, false).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/uk/cart", nil))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d for a failure the buyer cannot act on", recorder.Code, http.StatusInternalServerError)
	}
	for _, leaked := range []string{"cart_items", "relation", "pq:"} {
		if strings.Contains(recorder.Body.String(), leaked) {
			t.Fatalf("response leaks %q: %s", leaked, recorder.Body.String())
		}
	}
}

func TestKnownCartFailuresKeepTheirOwnStatus(t *testing.T) {
	// The classification must not flatten everything into 500: an invalid item
	// is the caller's to fix and has to stay distinguishable.
	for name, testCase := range map[string]struct {
		err    error
		status int
	}{
		"missing line":  {domain.ErrItemNotFound, http.StatusNotFound},
		"invalid item":  {domain.ErrInvalidItem, http.StatusUnprocessableEntity},
		"invalid owner": {domain.ErrInvalidOwner, http.StatusUnprocessableEntity},
	} {
		t.Run(name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			newCartRouter(&cartServiceFake{err: testCase.err}, false).
				ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/uk/cart", nil))
			if recorder.Code != testCase.status {
				t.Fatalf("status = %d, want %d", recorder.Code, testCase.status)
			}
		})
	}
}

func TestRegisterRoutesIsANoOpWithoutAService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterRoutes(router.Group("/api/:lang"), nil, false)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/uk/cart", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want the routes absent", recorder.Code)
	}
}

func sessionCookie(t *testing.T, cookies []*http.Cookie) *http.Cookie {
	t.Helper()
	for _, cookie := range cookies {
		if cookie.Name == cartowner.SessionCookie {
			return cookie
		}
	}
	return nil
}

func newCartRouter(service domain.Service, secureCookies bool, middleware ...gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/:lang")
	group.Use(middleware...)
	RegisterRoutes(group, service, secureCookies)
	return router
}

type cartServiceFake struct {
	calls     int
	lastOwner domain.Owner
	lastItem  domain.Item
	err       error
}

func (f *cartServiceFake) record(owner domain.Owner) (*domain.Cart, error) {
	f.calls++
	f.lastOwner = owner
	if f.err != nil {
		return nil, f.err
	}
	return &domain.Cart{ID: uuid.New()}, nil
}

func (f *cartServiceFake) GetOrCreate(_ context.Context, owner domain.Owner) (*domain.Cart, error) {
	return f.record(owner)
}
func (f *cartServiceFake) Add(_ context.Context, owner domain.Owner, item domain.Item) (*domain.Cart, error) {
	f.lastItem = item
	return f.record(owner)
}
func (f *cartServiceFake) SetQuantity(_ context.Context, owner domain.Owner, item domain.Item) (*domain.Cart, error) {
	f.lastItem = item
	return f.record(owner)
}
func (f *cartServiceFake) Remove(_ context.Context, owner domain.Owner, _ uuid.UUID) (*domain.Cart, error) {
	return f.record(owner)
}
func (f *cartServiceFake) SetPromoCode(_ context.Context, owner domain.Owner, _ string) (*domain.Cart, error) {
	return f.record(owner)
}

var _ domain.Service = (*cartServiceFake)(nil)
