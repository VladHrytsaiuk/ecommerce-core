package http

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	adminDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/apiresponse"
	supportApp "github.com/VladHrytsaiuk/ecommerce-core/internal/support/application"
	support "github.com/VladHrytsaiuk/ecommerce-core/internal/support/domain"
)

// Support tickets are reachable by guests, so the properties worth pinning are
// which subject a follow-up message is attributed to, that a guest can open a
// ticket at all, and that an infrastructure failure is not reported as if the
// caller had sent something wrong.

func TestAGuestCanOpenATicketWithTheirOwnEmail(t *testing.T) {
	world := newWorld(t)
	world.anonymous = true

	recorder := world.do(http.MethodPost, "/api/v1/support/tickets", `{"email":"Guest@Example.test","subject":"Where is my order","body":"It has not arrived."}`)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if world.repository.created.CustomerID != nil {
		t.Fatalf("guest ticket attributed to customer %v", world.repository.created.CustomerID)
	}
	if world.repository.created.Email != "guest@example.test" {
		t.Fatalf("stored email %q, want it normalised", world.repository.created.Email)
	}
}

func TestAnAuthenticatedTicketTakesTheEmailFromTheAccount(t *testing.T) {
	// The account is the trustworthy source; the body is not required to carry
	// one, and a signed-in customer should not have to retype it.
	world := newWorld(t)

	recorder := world.do(http.MethodPost, "/api/v1/support/tickets", `{"subject":"Question","body":"About my order."}`)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if world.repository.created.CustomerID == nil || *world.repository.created.CustomerID != world.customerID {
		t.Fatalf("ticket attributed to %v, want %s", world.repository.created.CustomerID, world.customerID)
	}
	if world.repository.created.Email != world.customers.email {
		t.Fatalf("stored email %q, want the account's %q", world.repository.created.Email, world.customers.email)
	}
}

func TestATicketIsRejectedWithoutAUsableEmailOrSubject(t *testing.T) {
	for name, body := range map[string]string{
		"no email":         `{"subject":"Hello","body":"Hi"}`,
		"malformed email":  `{"email":"not-an-address","subject":"Hello","body":"Hi"}`,
		"display name":     `{"email":"Someone <a@b.test>","subject":"Hello","body":"Hi"}`,
		"no subject":       `{"email":"a@b.test","body":"Hi"}`,
		"blank subject":    `{"email":"a@b.test","subject":"   ","body":"Hi"}`,
		"subject too long": `{"email":"a@b.test","subject":"` + strings.Repeat("s", 256) + `","body":"Hi"}`,
	} {
		t.Run(name, func(t *testing.T) {
			world := newWorld(t)
			world.anonymous = true // no account email to fall back on

			recorder := world.do(http.MethodPost, "/api/v1/support/tickets", body)

			if recorder.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusUnprocessableEntity, recorder.Body.String())
			}
			if world.repository.created.Subject != "" {
				t.Fatal("an invalid ticket was stored")
			}
		})
	}
}

func TestSpamProtectionRefusesTheTicketAsRateLimited(t *testing.T) {
	// Not a validation failure: the request is well formed and the caller is
	// being asked to slow down, which is what 429 says.
	world := newWorld(t)
	world.spam.err = support.ErrSpam

	recorder := world.do(http.MethodPost, "/api/v1/support/tickets", `{"email":"a@b.test","subject":"Hello","body":"Hi"}`)

	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusTooManyRequests, recorder.Body.String())
	}
	if world.repository.created.Subject != "" {
		t.Fatal("a ticket rejected as spam was stored")
	}
}

func TestAnOversizedTicketIsRefusedBeforeItIsRead(t *testing.T) {
	// Without the reader cap, an unauthenticated route would accept a body of
	// any size straight into memory.
	world := newWorld(t)
	world.anonymous = true
	body := fmt.Sprintf(`{"email":"a@b.test","subject":"Hello","body":%q}`, strings.Repeat("x", 64<<10))

	recorder := world.do(http.MethodPost, "/api/v1/support/tickets", body)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	if world.repository.created.Subject != "" {
		t.Fatal("an oversized ticket was stored")
	}
}

func TestAFollowUpMessageRequiresAnAuthenticatedSubject(t *testing.T) {
	// Guest tickets have no signed, single-purpose token yet, so an anonymous
	// caller who guessed a ticket ID must not be able to post into it.
	world := newWorld(t)
	world.anonymous = true

	recorder := world.do(http.MethodPost, "/api/v1/support/tickets/"+uuid.NewString()+"/messages", `{"body":"any update?"}`)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusUnauthorized, recorder.Body.String())
	}
	if world.repository.messagedTicket != uuid.Nil {
		t.Fatal("an anonymous caller reached the message repository")
	}
}

func TestAFollowUpMessageIsAttributedToTheTokenSubject(t *testing.T) {
	world := newWorld(t)
	ticketID := uuid.New()

	recorder := world.do(http.MethodPost, "/api/v1/support/tickets/"+ticketID.String()+"/messages", `{"body":"  any update?  "}`)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if world.repository.messagedTicket != ticketID || world.repository.messagedBy != world.customerID {
		t.Fatalf("message recorded on %s by %s", world.repository.messagedTicket, world.repository.messagedBy)
	}
	if world.repository.messagedBody != "any update?" {
		t.Fatalf("stored body %q, want it trimmed", world.repository.messagedBody)
	}
}

func TestAMessageOnSomeoneElsesTicketIsNotFound(t *testing.T) {
	// The repository scopes the lookup by customer, so another customer's
	// ticket is indistinguishable from one that does not exist.
	world := newWorld(t)
	world.repository.messageErr = support.ErrTicketNotFound

	recorder := world.do(http.MethodPost, "/api/v1/support/tickets/"+uuid.NewString()+"/messages", `{"body":"hello"}`)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusNotFound, recorder.Body.String())
	}
}

func TestAMessageOnAClosedTicketIsRefused(t *testing.T) {
	world := newWorld(t)
	world.repository.messageErr = support.ErrMessageForbidden

	recorder := world.do(http.MethodPost, "/api/v1/support/tickets/"+uuid.NewString()+"/messages", `{"body":"hello"}`)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusForbidden, recorder.Body.String())
	}
}

func TestAMalformedTicketIdentifierIsRejectedBeforeTheService(t *testing.T) {
	world := newWorld(t)

	recorder := world.do(http.MethodPost, "/api/v1/support/tickets/not-a-uuid/messages", `{"body":"hello"}`)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	if world.repository.messagedTicket != uuid.Nil {
		t.Fatal("a malformed identifier reached the repository")
	}
}

func TestEachAdminRouteRequiresItsOwnPermission(t *testing.T) {
	id := uuid.NewString()
	for name, route := range map[string]struct {
		method, path, body, permission string
	}{
		"list":       {http.MethodGet, "/api/v1/admin/support/tickets", "", PermissionSupportRead},
		"get":        {http.MethodGet, "/api/v1/admin/support/tickets/" + id, "", PermissionSupportRead},
		"reply":      {http.MethodPost, "/api/v1/admin/support/tickets/" + id + "/messages", `{"body":"on it"}`, PermissionSupportWrite},
		"transition": {http.MethodPatch, "/api/v1/admin/support/tickets/" + id + "/status", `{"status":"closed"}`, PermissionSupportWrite},
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

func TestTheAdminListingIsBounded(t *testing.T) {
	for name, expected := range map[string]struct {
		query         string
		limit, offset int
	}{
		"defaults":        {"", 20, 0},
		"second page":     {"?page=2&limit=5", 5, 5},
		"limit at cap":    {"?limit=100", 100, 0},
		"limit over cap":  {"?limit=101", 20, 0},
		"limit zero":      {"?limit=0", 20, 0},
		"page at cap":     {"?page=1000", 20, 19980},
		"page over cap":   {"?page=1001", 20, 0},
		"page zero":       {"?page=0", 20, 0},
		"page not number": {"?page=abc", 20, 0},
	} {
		t.Run(name, func(t *testing.T) {
			world := newWorld(t)

			recorder := world.do(http.MethodGet, "/api/v1/admin/support/tickets"+expected.query, "")

			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
			}
			if world.repository.limit != expected.limit || world.repository.offset != expected.offset {
				t.Fatalf("queried limit %d offset %d, want %d and %d", world.repository.limit, world.repository.offset, expected.limit, expected.offset)
			}
		})
	}
}

func TestAnAgentReplySchedulesTheCustomerEmailInTheSameTransaction(t *testing.T) {
	world := newWorld(t)

	recorder := world.do(http.MethodPost, "/api/v1/admin/support/tickets/"+uuid.NewString()+"/messages", `{"body":"  looking into it  "}`)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if world.repository.agentBody != "looking into it" {
		t.Fatalf("stored body %q, want it trimmed", world.repository.agentBody)
	}
	if world.scheduler.jobType != "support_agent_reply" || world.scheduler.email != world.repository.ticket.Email {
		t.Fatalf("scheduled %q to %q", world.scheduler.jobType, world.scheduler.email)
	}
	if !world.transactions.allCommitted() {
		t.Fatal("the reply and its notification did not share one committed transaction")
	}
}

func TestAnAgentReplyIsRolledBackWhenTheNotificationCannotBeScheduled(t *testing.T) {
	// The reply and the email that announces it are one unit: a stored reply
	// the customer is never told about is worse than a failed request.
	world := newWorld(t)
	world.scheduler.err = errors.New("scheduler unavailable")

	recorder := world.do(http.MethodPost, "/api/v1/admin/support/tickets/"+uuid.NewString()+"/messages", `{"body":"looking into it"}`)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusInternalServerError, recorder.Body.String())
	}
	if world.transactions.allCommitted() {
		t.Fatal("the transaction committed despite the notification failing")
	}
}

func TestAnEmptyOrOversizedAgentReplyIsRefused(t *testing.T) {
	for name, body := range map[string]string{
		"empty":    `{"body":""}`,
		"blank":    `{"body":"   "}`,
		"too long": fmt.Sprintf(`{"body":%q}`, strings.Repeat("r", 2001)),
		"not json": `{`,
		"no body":  `{}`,
	} {
		t.Run(name, func(t *testing.T) {
			world := newWorld(t)

			recorder := world.do(http.MethodPost, "/api/v1/admin/support/tickets/"+uuid.NewString()+"/messages", body)

			if recorder.Code/100 != 4 {
				t.Fatalf("status = %d, want a client error; body = %s", recorder.Code, recorder.Body.String())
			}
			if world.repository.agentBody != "" {
				t.Fatal("an invalid agent reply was stored")
			}
		})
	}
}

func TestAStatusTransitionPassesTheTrimmedTarget(t *testing.T) {
	world := newWorld(t)

	recorder := world.do(http.MethodPatch, "/api/v1/admin/support/tickets/"+uuid.NewString()+"/status", `{"status":"  closed  "}`)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if world.repository.transitionTo != "closed" {
		t.Fatalf("transitioned to %q, want it trimmed", world.repository.transitionTo)
	}
}

func TestAMissingTicketIsReportedAsNotFoundOnEveryAdminRoute(t *testing.T) {
	id := uuid.NewString()
	for name, route := range map[string]struct{ method, path, body string }{
		"get":        {http.MethodGet, "/api/v1/admin/support/tickets/" + id, ""},
		"reply":      {http.MethodPost, "/api/v1/admin/support/tickets/" + id + "/messages", `{"body":"on it"}`},
		"transition": {http.MethodPatch, "/api/v1/admin/support/tickets/" + id + "/status", `{"status":"closed"}`},
	} {
		t.Run(name, func(t *testing.T) {
			world := newWorld(t)
			world.repository.err = support.ErrTicketNotFound

			recorder := world.do(route.method, route.path, route.body)

			if recorder.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusNotFound, recorder.Body.String())
			}
		})
	}
}

func TestAnInfrastructureFailureIsNotReportedAsTheCallersFault(t *testing.T) {
	// Every admin error used to render as 422. A dropped database connection
	// told the operator their input was invalid, returned a 4xx that no alert
	// counts, and invited a retry that could not succeed.
	for name, route := range map[string]struct{ method, path, body string }{
		"list":       {http.MethodGet, "/api/v1/admin/support/tickets", ""},
		"get":        {http.MethodGet, "/api/v1/admin/support/tickets/" + uuid.NewString(), ""},
		"reply":      {http.MethodPost, "/api/v1/admin/support/tickets/" + uuid.NewString() + "/messages", `{"body":"on it"}`},
		"transition": {http.MethodPatch, "/api/v1/admin/support/tickets/" + uuid.NewString() + "/status", `{"status":"closed"}`},
	} {
		t.Run(name, func(t *testing.T) {
			world := newWorld(t)
			world.repository.err = errors.New("connection reset by peer")

			recorder := world.do(route.method, route.path, route.body)

			if recorder.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusInternalServerError, recorder.Body.String())
			}
			if strings.Contains(recorder.Body.String(), "connection reset") {
				t.Fatalf("the internal error leaked into the response: %s", recorder.Body.String())
			}
		})
	}
}

func TestAnInvalidAdminRequestIsStillAClientError(t *testing.T) {
	world := newWorld(t)
	world.repository.err = support.ErrInvalidTicket

	recorder := world.do(http.MethodPatch, "/api/v1/admin/support/tickets/"+uuid.NewString()+"/status", `{"status":"nowhere"}`)

	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusUnprocessableEntity, recorder.Body.String())
	}
}

// --- harness ---

type world struct {
	router       *gin.Engine
	repository   *repositoryStub
	spam         *spamStub
	customers    *customerEmailStub
	scheduler    *schedulerStub
	transactions *transactionStub
	authorizer   *authorizerStub
	customerID   uuid.UUID
	anonymous    bool
}

func newWorld(t *testing.T) *world {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := &world{
		repository:   &repositoryStub{},
		spam:         &spamStub{},
		customers:    &customerEmailStub{email: "customer@example.test"},
		scheduler:    &schedulerStub{},
		transactions: &transactionStub{},
		authorizer:   &authorizerStub{},
		customerID:   uuid.New(),
	}
	w.repository.ticket = support.Ticket{ID: uuid.New(), Email: "customer@example.test", Subject: "Question", Status: support.StatusNew}
	service := supportApp.NewService(w.repository, w.spam, w.customers).WithAdminWorkflow(w.transactions, w.scheduler)

	renderer := apiresponse.NewErrorRenderer(nil)
	w.router = gin.New()
	w.router.Use(renderer.Middleware())
	authenticate := func(c *gin.Context) {
		if !w.anonymous {
			c.Set("user_id", w.customerID)
		}
	}
	v1 := w.router.Group("/api/v1")
	RegisterV1Routes(v1, service, renderer, authenticate, authenticate)
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
	calls          int
	err            error
	ticket         support.Ticket
	created        support.Ticket
	messagedTicket uuid.UUID
	messagedBy     uuid.UUID
	messagedBody   string
	messageErr     error
	agentBody      string
	transitionTo   string
	limit, offset  int
}

func (r *repositoryStub) Create(_ context.Context, ticket support.Ticket, _ support.Message) (*support.Ticket, error) {
	r.calls++
	if r.err != nil {
		return nil, r.err
	}
	ticket.ID = uuid.New()
	r.created = ticket
	return &ticket, nil
}

func (r *repositoryStub) AddCustomerMessage(_ context.Context, ticketID, customerID uuid.UUID, body string) error {
	r.calls++
	if r.messageErr != nil {
		return r.messageErr
	}
	r.messagedTicket, r.messagedBy, r.messagedBody = ticketID, customerID, body
	return nil
}

func (r *repositoryStub) List(_ context.Context, _ string, limit, offset int) ([]support.Ticket, int64, error) {
	r.calls++
	if r.err != nil {
		return nil, 0, r.err
	}
	r.limit, r.offset = limit, offset
	return []support.Ticket{r.ticket}, 1, nil
}

func (r *repositoryStub) Get(context.Context, uuid.UUID) (*support.Ticket, []support.Message, error) {
	r.calls++
	if r.err != nil {
		return nil, nil, r.err
	}
	return &r.ticket, nil, nil
}

func (r *repositoryStub) AddAgentMessageAndSetPending(_ context.Context, _, _ uuid.UUID, body string) (*support.Ticket, error) {
	r.calls++
	if r.err != nil {
		return nil, r.err
	}
	r.agentBody = body
	return &r.ticket, nil
}

func (r *repositoryStub) TransitionStatus(_ context.Context, _ uuid.UUID, target string) (*support.Ticket, error) {
	r.calls++
	if r.err != nil {
		return nil, r.err
	}
	r.transitionTo = target
	return &r.ticket, nil
}

type spamStub struct{ err error }

func (s *spamStub) Check(context.Context, string, string, string) error { return s.err }

type customerEmailStub struct {
	email string
	err   error
}

func (c *customerEmailStub) EmailForCustomer(context.Context, uuid.UUID) (string, error) {
	return c.email, c.err
}

type schedulerStub struct {
	jobType, email string
	err            error
}

func (s *schedulerStub) ScheduleEmail(_ context.Context, jobType, _, email string, _ any) error {
	if s.err != nil {
		return s.err
	}
	s.jobType, s.email = jobType, email
	return nil
}

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

type authorizerStub struct {
	permission string
	err        error
}

func (a *authorizerStub) Require(_ context.Context, _ uuid.UUID, permission string) error {
	a.permission = permission
	return a.err
}
