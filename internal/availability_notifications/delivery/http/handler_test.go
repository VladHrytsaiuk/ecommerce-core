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

	availabilityApp "github.com/VladHrytsaiuk/ecommerce-core/internal/availability_notifications/application"
	availability "github.com/VladHrytsaiuk/ecommerce-core/internal/availability_notifications/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/apiresponse"
)

// A public endpoint that takes an email address. What matters is who it
// attributes a subscription to and what it refuses outright.

func TestAGuestSubscribesWithTheirOwnAddress(t *testing.T) {
	repository := &subscriptionRepositoryFake{created: true}
	variantID := uuid.New()

	recorder := post(t, repository, nil, uuid.Nil, variantID, `{"email":"Buyer@Example.COM"}`)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	// Normalized, or the same person subscribing twice with different casing
	// would be two subscribers and two emails.
	if repository.saved.Email != "buyer@example.com" {
		t.Fatalf("stored email = %q, want it lowercased", repository.saved.Email)
	}
	if repository.saved.VariantID != variantID {
		t.Fatalf("stored variant = %s, want %s", repository.saved.VariantID, variantID)
	}
}

func TestAnAuthenticatedCustomerNeedsNoAddress(t *testing.T) {
	// Their address is resolved server-side, so a signed-in shopper cannot
	// subscribe someone else by typing their address.
	customerID := uuid.New()
	repository := &subscriptionRepositoryFake{created: true}
	emails := &customerEmailFake{email: "member@example.com"}

	recorder := post(t, repository, emails, customerID, uuid.New(), `{}`)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if emails.asked != customerID {
		t.Fatalf("resolved the address for %s, want the authenticated customer %s", emails.asked, customerID)
	}
	if repository.saved.Email != "member@example.com" {
		t.Fatalf("stored email = %q, want the one resolved server-side", repository.saved.Email)
	}
}

func TestAnAnonymousRequestWithNoAddressIsRefused(t *testing.T) {
	repository := &subscriptionRepositoryFake{}

	recorder := post(t, repository, nil, uuid.Nil, uuid.New(), `{}`)

	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnprocessableEntity)
	}
	if repository.calls != 0 {
		t.Fatal("a subscription with no address reached the repository")
	}
}

func TestAMalformedVariantOrBodyIsRefusedBeforeTheService(t *testing.T) {
	for name, testCase := range map[string]struct{ variant, body string }{
		"variant not a uuid": {"not-a-uuid", `{"email":"buyer@example.com"}`},
		"body not json":      {uuid.NewString(), `nonsense`},
		"email too long":     {uuid.NewString(), `{"email":"` + strings.Repeat("a", 321) + `"}`},
	} {
		t.Run(name, func(t *testing.T) {
			repository := &subscriptionRepositoryFake{}
			recorder := postRaw(t, repository, nil, uuid.Nil, testCase.variant, testCase.body)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
			}
			if repository.calls != 0 {
				t.Fatal("a malformed request reached the repository")
			}
		})
	}
}

func TestResubscribingIsNotTreatedAsNew(t *testing.T) {
	// 200 rather than 201, so a client can tell "you are already on the list"
	// from "you have just been added".
	repository := &subscriptionRepositoryFake{created: false}

	recorder := post(t, repository, nil, uuid.Nil, uuid.New(), `{"email":"buyer@example.com"}`)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d for an existing subscription", recorder.Code, http.StatusOK)
	}
}

func post(t *testing.T, repository availability.Repository, emails availability.CustomerEmailReader, customerID, variantID uuid.UUID, body string) *httptest.ResponseRecorder {
	t.Helper()
	return postRaw(t, repository, emails, customerID, variantID.String(), body)
}

func postRaw(t *testing.T, repository availability.Repository, emails availability.CustomerEmailReader, customerID uuid.UUID, variant, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	renderer := apiresponse.NewErrorRenderer(nil)
	router := gin.New()
	router.Use(renderer.Middleware())
	var service *availabilityApp.Service
	if emails != nil {
		service = availabilityApp.NewService(repository, emails)
	} else {
		service = availabilityApp.NewService(repository)
	}
	var optionalAuth gin.HandlerFunc
	if customerID != uuid.Nil {
		optionalAuth = func(c *gin.Context) { c.Set("user_id", customerID) }
	}
	RegisterV1Routes(router.Group("/api/v1"), service, renderer, optionalAuth)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/catalog/variants/"+variant+"/subscribe", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	return recorder
}

type subscriptionRepositoryFake struct {
	calls   int
	created bool
	saved   availability.Subscription
}

func (f *subscriptionRepositoryFake) Create(_ context.Context, subscription availability.Subscription) (*availability.Subscription, bool, error) {
	f.calls++
	f.saved = subscription
	return &subscription, f.created, nil
}

func (f *subscriptionRepositoryFake) MarkPendingNotified(context.Context, uuid.UUID, time.Time) ([]availability.Subscription, error) {
	return nil, nil
}

type customerEmailFake struct {
	asked uuid.UUID
	email string
}

func (f *customerEmailFake) EmailForCustomer(_ context.Context, customerID uuid.UUID) (string, error) {
	f.asked = customerID
	return f.email, nil
}
