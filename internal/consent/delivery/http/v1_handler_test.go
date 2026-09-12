package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	adminDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/domain"
	consentApp "github.com/VladHrytsaiuk/ecommerce-core/internal/consent/application"
	consent "github.com/VladHrytsaiuk/ecommerce-core/internal/consent/domain"
	coreEvents "github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/apiresponse"
)

// These routes carry GDPR obligations, so the properties worth pinning are
// whose consent is read and written, and that the store never claims to have
// done something it cannot do.

func TestConsentsAreReadAndWrittenForTheAuthenticatedCustomerOnly(t *testing.T) {
	world := newWorld(t)

	if recorder := world.do(http.MethodGet, "/api/v1/customers/me/consents", ""); recorder.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if world.repository.readFor != world.customerID {
		t.Fatalf("read consents for %s, want the authenticated customer %s", world.repository.readFor, world.customerID)
	}

	// The body names someone else; the token decides.
	body := fmt.Sprintf(`{"document_type":"marketing","version":"1.0","customer_id":%q}`, uuid.New())
	if recorder := world.do(http.MethodPost, "/api/v1/customers/me/consents", body); recorder.Code != http.StatusNoContent {
		t.Fatalf("grant status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if world.repository.granted.CustomerID != world.customerID {
		t.Fatalf("granted consent for %s, want %s", world.repository.granted.CustomerID, world.customerID)
	}
	if world.repository.granted.DocumentType != "marketing" || world.repository.granted.DocumentVersion != "1.0" {
		t.Fatalf("granted = %+v", world.repository.granted)
	}
}

func TestAnAnonymousCallerReachesNoCustomerConsentRoute(t *testing.T) {
	for name, route := range map[string]struct{ method, path, body string }{
		"list consents":   {http.MethodGet, "/api/v1/customers/me/consents", ""},
		"grant consent":   {http.MethodPost, "/api/v1/customers/me/consents", `{"document_type":"marketing","version":"1.0"}`},
		"withdraw":        {http.MethodDelete, "/api/v1/customers/me/consents/marketing", ""},
		"privacy request": {http.MethodPost, "/api/v1/customers/me/privacy-requests", `{"request_type":"export"}`},
	} {
		t.Run(name, func(t *testing.T) {
			world := newWorld(t)
			world.anonymous = true

			recorder := world.do(route.method, route.path, route.body)

			if recorder.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusUnauthorized, recorder.Body.String())
			}
			if world.repository.calls != 0 {
				t.Fatal("an anonymous request reached the consent repository")
			}
		})
	}
}

func TestTheActiveDocumentListIsPublic(t *testing.T) {
	// A visitor must be able to read the terms before having an account.
	world := newWorld(t)
	world.anonymous = true

	recorder := world.do(http.MethodGet, "/api/v1/legal/documents/active", "")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
}

func TestConsentToAnUnpublishedDocumentIsRefused(t *testing.T) {
	// Recording consent against a version that was never published leaves an
	// audit trail pointing at a document nobody agreed to.
	world := newWorld(t)
	world.repository.activeDocument = false

	recorder := world.do(http.MethodPost, "/api/v1/customers/me/consents", `{"document_type":"marketing","version":"9.9"}`)

	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusUnprocessableEntity, recorder.Body.String())
	}
	if world.repository.granted.ID != uuid.Nil {
		t.Fatal("consent was recorded against an inactive document")
	}
}

func TestTermsCannotBeWithdrawnWhileOrdersAreActive(t *testing.T) {
	// Withdrawing terms mid-order would leave the store processing an order
	// under terms the customer no longer accepts.
	//
	// The path parameter is attacker-controlled, so the guard has to key off
	// the same value the repository is eventually given: comparing the raw
	// parameter while writing the trimmed one let "%20terms" walk straight
	// past this check.
	for name, documentType := range map[string]string{
		"plain":           "terms",
		"leading space":   "%20terms",
		"trailing space":  "terms%20",
		"surrounded":      "%20terms%20",
		"tab":             "%09terms",
		"trailing return": "terms%0D",
	} {
		t.Run(name, func(t *testing.T) {
			world := newWorld(t)
			world.orders.active = true

			recorder := world.do(http.MethodDelete, "/api/v1/customers/me/consents/"+documentType, "")

			if recorder.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusUnprocessableEntity, recorder.Body.String())
			}
			if world.repository.withdrewType != "" {
				t.Fatalf("withdrew %q despite active orders", world.repository.withdrewType)
			}
		})
	}
}

func TestTermsCanBeWithdrawnOnceNoOrderIsActive(t *testing.T) {
	world := newWorld(t)

	recorder := world.do(http.MethodDelete, "/api/v1/customers/me/consents/terms", "")

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusNoContent, recorder.Body.String())
	}
	if world.repository.withdrewType != "terms" || world.repository.withdrewFor != world.customerID {
		t.Fatalf("withdrew %q for %s", world.repository.withdrewType, world.repository.withdrewFor)
	}
}

func TestWithdrawingAnyOtherConsentDoesNotConsultOrders(t *testing.T) {
	world := newWorld(t)
	world.orders.active = true

	recorder := world.do(http.MethodDelete, "/api/v1/customers/me/consents/marketing", "")

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusNoContent, recorder.Body.String())
	}
	if world.repository.withdrewType != "marketing" {
		t.Fatalf("withdrew %q, want marketing", world.repository.withdrewType)
	}
}

func TestAnErasureRequestIsRefusedWhereNothingCanCarryItOut(t *testing.T) {
	// Accepting it would put a promise in the queue that nothing drains.
	world := newWorld(t)

	recorder := world.do(http.MethodPost, "/api/v1/customers/me/privacy-requests", `{"request_type":"erasure"}`)

	if recorder.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusNotImplemented, recorder.Body.String())
	}
	if world.repository.privacyRequest.ID != uuid.Nil {
		t.Fatal("an erasure request was queued with no executor configured")
	}
}

func TestAnErasureRequestIsAcceptedWhereItCanBeCarriedOut(t *testing.T) {
	world := newWorld(t, withErasure)

	recorder := world.do(http.MethodPost, "/api/v1/customers/me/privacy-requests", `{"request_type":"erasure"}`)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusAccepted, recorder.Body.String())
	}
	if world.repository.privacyRequest.RequestType != "erasure" || world.repository.privacyRequest.Status != "pending" {
		t.Fatalf("queued request = %+v", world.repository.privacyRequest)
	}
	if world.repository.privacyRequest.CustomerID != world.customerID {
		t.Fatalf("queued for %s, want %s", world.repository.privacyRequest.CustomerID, world.customerID)
	}
}

func TestAnExportRequestIsRefusedWhereNothingCanProduceIt(t *testing.T) {
	// This asserted a 202 before, which was the defect: the request was stored,
	// approved, moved to in_progress and never answered. The right of access
	// gets the same treatment as the right to erasure.
	world := newWorld(t)

	recorder := world.do(http.MethodPost, "/api/v1/customers/me/privacy-requests", `{"request_type":"export"}`)

	if recorder.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusNotImplemented, recorder.Body.String())
	}
	if world.repository.privacyRequest.ID != uuid.Nil {
		t.Fatal("an export request was queued with no exporter configured")
	}
}

func TestAnExportRequestIsAcceptedWhereItCanBeProduced(t *testing.T) {
	world := newWorld(t, withExport)

	recorder := world.do(http.MethodPost, "/api/v1/customers/me/privacy-requests", `{"request_type":"export"}`)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusAccepted, recorder.Body.String())
	}
	if world.repository.privacyRequest.RequestType != "export" || world.repository.privacyRequest.CustomerID != world.customerID {
		t.Fatalf("queued request = %+v", world.repository.privacyRequest)
	}
}

func TestAnUnknownPrivacyRequestTypeIsRefused(t *testing.T) {
	for _, requestType := range []string{"", "delete", "erasure ", "EXPORT", "rectification"} {
		t.Run(requestType, func(t *testing.T) {
			world := newWorld(t, withErasure)

			recorder := world.do(http.MethodPost, "/api/v1/customers/me/privacy-requests", fmt.Sprintf(`{"request_type":%q}`, requestType))

			if recorder.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusUnprocessableEntity, recorder.Body.String())
			}
			if world.repository.privacyRequest.ID != uuid.Nil {
				t.Fatalf("queued an unknown request type %q", requestType)
			}
		})
	}
}

// A guest has no account, so the signed link is the only thing that can
// authorise them to stop receiving marketing. Before this endpoint existed they
// had no implemented way out at all.

func TestASignedTokenWithdrawsMarketingConsent(t *testing.T) {
	world := newWorld(t)
	world.anonymous = true // a guest, by definition
	token, err := world.signer.Sign("guest@example.test", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}

	recorder := world.do(http.MethodPost, "/api/v1/consent/unsubscribe", fmt.Sprintf(`{"token":%q}`, token))

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusNoContent, recorder.Body.String())
	}
	if world.repository.withdrewMarketingFor != "guest@example.test" {
		t.Fatalf("withdrew for %q, want the address the token names", world.repository.withdrewMarketingFor)
	}
}

func TestATokenForOneAddressCannotUnsubscribeAnother(t *testing.T) {
	world := newWorld(t)
	valid, err := world.signer.Sign("guest@example.test", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	payload, _, _ := strings.Cut(valid, ".")
	forged := payload + ".AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

	recorder := world.do(http.MethodPost, "/api/v1/consent/unsubscribe", fmt.Sprintf(`{"token":%q}`, forged))

	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusUnprocessableEntity, recorder.Body.String())
	}
	if world.repository.withdrewMarketingFor != "" {
		t.Fatalf("a forged token withdrew consent for %q", world.repository.withdrewMarketingFor)
	}
}

func TestAMalformedUnsubscribeRequestIsRejected(t *testing.T) {
	for name, body := range map[string]string{
		"no token":  `{}`,
		"empty":     `{"token":""}`,
		"not json":  `{`,
		"oversized": `{"token":"` + strings.Repeat("a", 513) + `"}`,
	} {
		t.Run(name, func(t *testing.T) {
			world := newWorld(t)
			recorder := world.do(http.MethodPost, "/api/v1/consent/unsubscribe", body)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
			}
			if world.repository.withdrewMarketingFor != "" {
				t.Fatal("a malformed request reached the repository")
			}
		})
	}
}

func TestTheUnsubscribeRouteRefusesGet(t *testing.T) {
	// Mail security scanners follow links. A GET that withdraws consent would
	// let a provider's link check unsubscribe the customer for them.
	world := newWorld(t)

	recorder := world.do(http.MethodGet, "/api/v1/consent/unsubscribe", "")

	if recorder.Code == http.StatusNoContent || recorder.Code == http.StatusOK {
		t.Fatalf("GET on the unsubscribe route returned %d", recorder.Code)
	}
}

func TestAdminLegalAndPrivacyRoutesRequireTheirOwnPermission(t *testing.T) {
	for name, route := range map[string]struct {
		method, path, body, permission string
	}{
		"create document":  {http.MethodPost, "/api/v1/admin/legal/documents", `{"type":"terms","version":"1.0","content_url":"https://example.test/terms"}`, PermissionLegalWrite},
		"publish document": {http.MethodPost, "/api/v1/admin/legal/documents/" + uuid.NewString() + "/publish", `{}`, PermissionLegalWrite},
		"list privacy":     {http.MethodGet, "/api/v1/admin/privacy-requests", "", PermissionPrivacyWrite},
		"approve privacy":  {http.MethodPost, "/api/v1/admin/privacy-requests/" + uuid.NewString() + "/approve", `{}`, PermissionPrivacyWrite},
	} {
		t.Run(name, func(t *testing.T) {
			world := newWorld(t)
			world.authorizer.err = adminDomain.ErrPermissionDenied

			recorder := world.do(route.method, route.path, route.body)

			if recorder.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusForbidden, recorder.Body.String())
			}
			if world.authorizer.permission != route.permission {
				t.Fatalf("checked %q, want %q", world.authorizer.permission, route.permission)
			}
			if world.repository.calls != 0 {
				t.Fatal("a denied admin request reached the repository")
			}
		})
	}
}

func TestApprovingAnErasureIsRefusedWhereNothingCanCarryItOut(t *testing.T) {
	// The approval is rolled back, so the request stays pending and visible
	// rather than being closed as done when nothing was erased.
	world := newWorld(t)
	world.repository.privacyRequest = consent.PrivacyRequest{ID: uuid.New(), CustomerID: world.customerID, RequestType: "erasure", Status: "pending"}

	recorder := world.do(http.MethodPost, "/api/v1/admin/privacy-requests/"+world.repository.privacyRequest.ID.String()+"/approve", `{}`)

	if recorder.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusNotImplemented, recorder.Body.String())
	}
	if world.eraser.calls != 0 {
		t.Fatal("erasure ran with no executor configured")
	}
}

func TestApprovingAnErasureErasesAndAuditsInOneTransaction(t *testing.T) {
	world := newWorld(t, withErasure)
	world.repository.privacyRequest = consent.PrivacyRequest{ID: uuid.New(), CustomerID: world.customerID, RequestType: "erasure", Status: "pending"}

	recorder := world.do(http.MethodPost, "/api/v1/admin/privacy-requests/"+world.repository.privacyRequest.ID.String()+"/approve", `{}`)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusAccepted, recorder.Body.String())
	}
	if world.eraser.erased != world.customerID {
		t.Fatalf("erased %s, want %s", world.eraser.erased, world.customerID)
	}
	if len(world.publisher.events) != 1 {
		t.Fatalf("audit events = %d, want exactly one", len(world.publisher.events))
	}
	if !world.transactions.allCommitted() {
		t.Fatal("the erasure and its audit event did not share one committed transaction")
	}
}

func TestAdminPrivacyListingIsBounded(t *testing.T) {
	// An unbounded limit reads the whole table and a deep page turns into a
	// large offset scan, both on an authenticated admin route.
	for name, query := range map[string]struct {
		query         string
		limit, offset int
	}{
		"defaults":        {"", 20, 0},
		"second page":     {"?page=2&limit=5", 5, 5},
		"limit at cap":    {"?limit=100", 100, 0},
		"limit over cap":  {"?limit=101", 20, 0},
		"limit zero":      {"?limit=0", 20, 0},
		"limit negative":  {"?limit=-5", 20, 0},
		"page zero":       {"?page=0", 20, 0},
		"page negative":   {"?page=-3", 20, 0},
		"page not number": {"?page=abc", 20, 0},
	} {
		t.Run(name, func(t *testing.T) {
			world := newWorld(t)

			recorder := world.do(http.MethodGet, "/api/v1/admin/privacy-requests"+query.query, "")

			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
			}
			if world.repository.limit != query.limit || world.repository.offset != query.offset {
				t.Fatalf("queried limit %d offset %d, want %d and %d", world.repository.limit, world.repository.offset, query.limit, query.offset)
			}
		})
	}
}

func TestACreatedLegalDocumentIsNotActiveUntilPublished(t *testing.T) {
	world := newWorld(t)

	recorder := world.do(http.MethodPost, "/api/v1/admin/legal/documents", `{"type":" terms ","version":" 2.0 ","content_url":" https://example.test/terms "}`)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	created := world.repository.createdDocument
	if created.IsActive {
		t.Fatal("a newly created legal document is already active; publishing is what makes it binding")
	}
	if created.Type != "terms" || created.Version != "2.0" || created.ContentURL != "https://example.test/terms" {
		t.Fatalf("stored document = %+v, want the fields trimmed", created)
	}
}

func TestAnIncompleteLegalDocumentIsRefused(t *testing.T) {
	for name, body := range map[string]string{
		"no type":    `{"version":"1.0","content_url":"https://example.test/terms"}`,
		"no version": `{"type":"terms","content_url":"https://example.test/terms"}`,
		"no url":     `{"type":"terms","version":"1.0"}`,
		"blank type": `{"type":"   ","version":"1.0","content_url":"https://example.test/terms"}`,
		"not json":   `{`,
	} {
		t.Run(name, func(t *testing.T) {
			world := newWorld(t)

			recorder := world.do(http.MethodPost, "/api/v1/admin/legal/documents", body)

			if recorder.Code/100 != 4 {
				t.Fatalf("status = %d, want a client error; body = %s", recorder.Code, recorder.Body.String())
			}
			if world.repository.createdDocument.ID != uuid.Nil {
				t.Fatal("an incomplete legal document was stored")
			}
		})
	}
}

func TestAMalformedDocumentIdentifierIsRejectedBeforeTheService(t *testing.T) {
	world := newWorld(t)

	recorder := world.do(http.MethodPost, "/api/v1/admin/legal/documents/not-a-uuid/publish", `{}`)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	if world.repository.calls != 0 {
		t.Fatal("a malformed identifier reached the repository")
	}
}

// --- harness ---

type world struct {
	router       *gin.Engine
	repository   *repositoryStub
	orders       *ordersStub
	authorizer   *authorizerStub
	publisher    *publisherStub
	eraser       *eraserStub
	exporter     *exporterStub
	signer       *consent.UnsubscribeSigner
	transactions *transactionStub
	customerID   uuid.UUID
	anonymous    bool
}

type option func(*world, *consentApp.Service) *consentApp.Service

// withErasure gives the deployment an erasure implementation, which is what
// separates a store that can honour a deletion request from one that cannot.
func withErasure(w *world, s *consentApp.Service) *consentApp.Service { return s.WithErasure(w.eraser) }

// withExport gives the deployment a way to answer a right-of-access request.
func withExport(w *world, s *consentApp.Service) *consentApp.Service { return s.WithExport(w.exporter) }

func newWorld(t *testing.T, options ...option) *world {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := &world{
		repository:   &repositoryStub{activeDocument: true},
		orders:       &ordersStub{},
		authorizer:   &authorizerStub{},
		publisher:    &publisherStub{},
		eraser:       &eraserStub{},
		exporter:     &exporterStub{},
		transactions: &transactionStub{},
		customerID:   uuid.New(),
	}
	signer, err := consent.NewUnsubscribeSigner("test-application-secret", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	w.signer = signer
	service := consentApp.NewService(w.repository, w.orders).
		WithAdminWorkflow(w.transactions, w.publisher).
		WithUnsubscribeSigner(signer)
	for _, apply := range options {
		service = apply(w, service)
	}

	renderer := apiresponse.NewErrorRenderer(nil)
	w.router = gin.New()
	w.router.Use(renderer.Middleware())
	authenticate := func(c *gin.Context) {
		if !w.anonymous {
			c.Set("user_id", w.customerID)
		}
	}
	v1 := w.router.Group("/api/v1")
	RegisterV1Routes(v1, service, authenticate, renderer)
	admin := v1.Group("/admin")
	admin.Use(authenticate)
	RegisterV1AdminRoutes(admin, w.authorizer, service, renderer)
	return w
}

func (w *world) do(method, path, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	w.router.ServeHTTP(recorder, request)
	return recorder
}

type repositoryStub struct {
	calls                int
	activeDocument       bool
	readFor              uuid.UUID
	granted              consent.CustomerConsent
	withdrewFor          uuid.UUID
	withdrewType         string
	withdrewMarketingFor string
	privacyRequest       consent.PrivacyRequest
	createdDocument      consent.LegalDocument
	completed            bool
	limit, offset        int
}

func (r *repositoryStub) ActiveDocuments(context.Context) ([]consent.LegalDocument, error) {
	r.calls++
	return []consent.LegalDocument{{ID: uuid.New(), Type: "terms", Version: "1.0", IsActive: true}}, nil
}

func (r *repositoryStub) Consents(_ context.Context, id uuid.UUID) ([]consent.CustomerConsent, error) {
	r.calls++
	r.readFor = id
	return nil, nil
}

func (r *repositoryStub) Grant(_ context.Context, value consent.CustomerConsent) error {
	r.calls++
	r.granted = value
	return nil
}

func (r *repositoryStub) Withdraw(_ context.Context, id uuid.UUID, documentType string, _ time.Time) error {
	r.calls++
	r.withdrewFor, r.withdrewType = id, documentType
	return nil
}

func (r *repositoryStub) WithdrawMarketingByEmail(_ context.Context, email string, _ time.Time) error {
	r.calls++
	r.withdrewMarketingFor = email
	return nil
}

func (r *repositoryStub) CreatePrivacyRequest(_ context.Context, value consent.PrivacyRequest) error {
	r.calls++
	r.privacyRequest = value
	return nil
}

func (r *repositoryStub) IsActiveDocument(context.Context, string, string) (bool, error) {
	r.calls++
	return r.activeDocument, nil
}

func (r *repositoryStub) CreateDocument(_ context.Context, value consent.LegalDocument) error {
	r.calls++
	r.createdDocument = value
	return nil
}

func (r *repositoryStub) PublishDocument(_ context.Context, id uuid.UUID) (*consent.LegalDocument, error) {
	r.calls++
	return &consent.LegalDocument{ID: id, Type: "terms", Version: "1.0", IsActive: true}, nil
}

func (r *repositoryStub) ListPrivacyRequests(_ context.Context, limit, offset int) ([]consent.PrivacyRequest, int64, error) {
	r.calls++
	r.limit, r.offset = limit, offset
	return nil, 0, nil
}

func (r *repositoryStub) CompletePrivacyRequest(_ context.Context, id uuid.UUID) error {
	if r.privacyRequest.ID != id {
		return consent.ErrInvalidPrivacyTransition
	}
	r.completed = true
	return nil
}

func (r *repositoryStub) ApprovePrivacyRequest(_ context.Context, id uuid.UUID) (*consent.PrivacyRequest, error) {
	r.calls++
	if r.privacyRequest.ID != id {
		return nil, consent.ErrInvalidPrivacyTransition
	}
	approved := r.privacyRequest
	approved.Status = "in_progress"
	return &approved, nil
}

type ordersStub struct{ active bool }

func (o *ordersStub) HasActiveOrders(context.Context, uuid.UUID) (bool, error) { return o.active, nil }

// transactionStub records whether each transaction it opened committed, so a
// test can tell a rolled-back approval from a committed one.
type transactionStub struct{ outcomes []error }

func (t *transactionStub) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	err := fn(ctx)
	t.outcomes = append(t.outcomes, err)
	return err
}

func (t *transactionStub) allCommitted() bool {
	if len(t.outcomes) == 0 {
		return false
	}
	for _, err := range t.outcomes {
		if err != nil {
			return false
		}
	}
	return true
}

type publisherStub struct{ events []coreEvents.DomainEvent }

func (p *publisherStub) Publish(_ context.Context, event coreEvents.DomainEvent) error {
	p.events = append(p.events, event)
	return nil
}

type eraserStub struct {
	calls  int
	erased uuid.UUID
	err    error
}

func (e *eraserStub) Erase(_ context.Context, customerID uuid.UUID) error {
	e.calls++
	e.erased = customerID
	return e.err
}

type exporterStub struct {
	calls    int
	exported uuid.UUID
}

func (e *exporterStub) Export(_ context.Context, customerID uuid.UUID) error {
	e.calls++
	e.exported = customerID
	return nil
}

type authorizerStub struct {
	permission string
	err        error
}

func (a *authorizerStub) Require(_ context.Context, _ uuid.UUID, permission string) error {
	a.permission = permission
	return a.err
}

var _ = json.Marshal
