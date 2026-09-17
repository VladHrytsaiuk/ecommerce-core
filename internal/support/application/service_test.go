package application

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	support "github.com/VladHrytsaiuk/ecommerce-core/internal/support/domain"
)

// Service had eight functions and no test. Its HTTP handler was tested against
// a fake service and its repository against real SQL, so what neither covered
// was everything in between: where a guest's address comes from, what reaches
// support_messages.body, and whether the agent's reply and the customer's mail
// are one transaction or two.

func TestCreateTicketTrimsAndStoresWhatItValidated(t *testing.T) {
	world := newSupportWorld()

	ticket, err := world.service.CreateTicket(context.Background(), nil, " Guest@Example.COM ", "  Broken link  ", "  the checkout button does nothing  ", "203.0.113.7")

	if err != nil {
		t.Fatalf("CreateTicket() error = %v", err)
	}
	if ticket == nil {
		t.Fatal("CreateTicket() returned no ticket")
	}
	if world.repository.createdTicket.Email != "guest@example.com" {
		t.Fatalf("stored email = %q, want it lowercased and trimmed", world.repository.createdTicket.Email)
	}
	if world.repository.createdTicket.Subject != "Broken link" {
		t.Fatalf("stored subject = %q, want it trimmed", world.repository.createdTicket.Subject)
	}
	// The value that was validated must be the value that was stored.
	if world.repository.createdMessage.Body != "the checkout button does nothing" {
		t.Fatalf("stored body = %q, want the trimmed body", world.repository.createdMessage.Body)
	}
	if world.repository.createdTicket.Status != support.StatusNew {
		t.Fatalf("stored status = %q, want %q", world.repository.createdTicket.Status, support.StatusNew)
	}
}

func TestCreateTicketResolvesAnAuthenticatedCustomersAddressServerSide(t *testing.T) {
	// A signed-in customer must not be able to open a ticket against somebody
	// else's address by supplying it in the payload.
	world := newSupportWorld()
	world.customers.email = "Registered@Example.com"
	customerID := uuid.New()

	if _, err := world.service.CreateTicket(context.Background(), &customerID, "", "Subject", "Body", "203.0.113.7"); err != nil {
		t.Fatalf("CreateTicket() error = %v", err)
	}
	if world.customers.asked != customerID {
		t.Fatalf("looked up %s, want the authenticated customer %s", world.customers.asked, customerID)
	}
	if world.repository.createdTicket.Email != "registered@example.com" {
		t.Fatalf("stored email = %q, want the address resolved from the customer record", world.repository.createdTicket.Email)
	}
}

func TestCreateTicketRefusesWhatCannotBeAnswered(t *testing.T) {
	longSubject := strings.Repeat("s", support.MaxSubjectRunes+1)
	longBody := strings.Repeat("b", support.MaxCustomerMessageRunes+1)
	for name, testCase := range map[string]struct{ email, subject, body string }{
		"no address":          {"", "Subject", "Body"},
		"malformed address":   {"not-an-address", "Subject", "Body"},
		"address with a name": {"Guest <guest@example.com>", "Subject", "Body"},
		"blank subject":       {"guest@example.com", "   ", "Body"},
		"oversized subject":   {"guest@example.com", longSubject, "Body"},
		"blank body":          {"guest@example.com", "Subject", "   "},
		// The bound that was missing: nothing below the HTTP handler stopped
		// an unbounded body reaching a TEXT column.
		"oversized body": {"guest@example.com", "Subject", longBody},
	} {
		t.Run(name, func(t *testing.T) {
			world := newSupportWorld()
			_, err := world.service.CreateTicket(context.Background(), nil, testCase.email, testCase.subject, testCase.body, "203.0.113.7")
			if !errors.Is(err, support.ErrInvalidTicket) {
				t.Fatalf("CreateTicket() error = %v, want ErrInvalidTicket", err)
			}
			if world.repository.createdTicket.Email != "" {
				t.Fatal("a refused ticket reached the repository")
			}
		})
	}
}

func TestCreateTicketStopsAtTheSpamProtector(t *testing.T) {
	world := newSupportWorld()
	world.spam.err = support.ErrSpam

	_, err := world.service.CreateTicket(context.Background(), nil, "guest@example.com", "Subject", "Body", "203.0.113.7")

	if !errors.Is(err, support.ErrSpam) {
		t.Fatalf("CreateTicket() error = %v, want ErrSpam", err)
	}
	if world.repository.createdTicket.Email != "" {
		t.Fatal("a rejected request still created a ticket")
	}
}

func TestAddCustomerMessageBoundsTheBodyBeforeTheSpamCheck(t *testing.T) {
	world := newSupportWorld()

	err := world.service.AddCustomerMessage(context.Background(), uuid.New(), uuid.New(),
		strings.Repeat("b", support.MaxCustomerMessageRunes+1), "203.0.113.7", "guest@example.com")

	if !errors.Is(err, support.ErrInvalidTicket) {
		t.Fatalf("AddCustomerMessage() error = %v, want ErrInvalidTicket", err)
	}
	if world.spam.calls != 0 {
		t.Fatal("an oversized body was handed to the spam protector")
	}
}

func TestAddCustomerMessageRefusesAnUnidentifiedSender(t *testing.T) {
	// Ownership is checked against the authenticated subject; without one there
	// is nothing to check, so this must not reach the repository.
	world := newSupportWorld()

	err := world.service.AddCustomerMessage(context.Background(), uuid.New(), uuid.Nil, "Body", "203.0.113.7", "guest@example.com")

	if !errors.Is(err, support.ErrMessageForbidden) {
		t.Fatalf("AddCustomerMessage() error = %v, want ErrMessageForbidden", err)
	}
	if world.repository.customerMessages != 0 {
		t.Fatal("a message with no sender reached the repository")
	}
}

func TestAnAgentReplyAndItsCustomerMailAreOneTransaction(t *testing.T) {
	// The reply is what the mail announces. Committing one without the other
	// either tells a customer about a reply that does not exist, or leaves a
	// reply nobody is told about.
	world := newSupportWorld()
	world.repository.ticket = &support.Ticket{ID: uuid.New(), Email: "guest@example.com", Subject: "Broken link"}

	ticket, err := world.service.ReplyAsAgent(context.Background(), world.repository.ticket.ID, uuid.New(), "  we have fixed it  ")

	if err != nil {
		t.Fatalf("ReplyAsAgent() error = %v", err)
	}
	if ticket == nil || !world.tx.committed {
		t.Fatalf("ticket = %v, committed = %v, want a committed reply", ticket, world.tx.committed)
	}
	if !world.scheduler.insideTransaction {
		t.Fatal("the customer mail was scheduled outside the reply's transaction")
	}
	if world.scheduler.template != "support_agent_reply" {
		t.Fatalf("scheduled %q, want the agent reply template", world.scheduler.template)
	}
	if world.scheduler.recipient != "guest@example.com" {
		t.Fatalf("mailed %q, want the ticket's address", world.scheduler.recipient)
	}
}

func TestAFailedMailRollsTheAgentReplyBack(t *testing.T) {
	world := newSupportWorld()
	world.repository.ticket = &support.Ticket{ID: uuid.New(), Email: "guest@example.com"}
	world.scheduler.err = errors.New("scheduler unavailable")

	_, err := world.service.ReplyAsAgent(context.Background(), world.repository.ticket.ID, uuid.New(), "we have fixed it")

	if err == nil {
		t.Fatal("ReplyAsAgent() committed a reply whose notification failed")
	}
	if world.tx.committed {
		t.Fatal("the transaction committed despite the failure")
	}
}

func TestReplyAsAgentRefusesWithoutTheAdminWorkflow(t *testing.T) {
	// Built by NewService alone, the service has neither a transaction manager
	// nor a scheduler. Replying would then write a reply and silently send no
	// mail, so it is refused instead.
	world := newSupportWorld()
	bare := NewService(world.repository, world.spam, world.customers)

	if _, err := bare.ReplyAsAgent(context.Background(), uuid.New(), uuid.New(), "Body"); !errors.Is(err, support.ErrInvalidTicket) {
		t.Fatalf("ReplyAsAgent() error = %v, want ErrInvalidTicket", err)
	}
}

func TestReplyAsAgentBoundsItsBody(t *testing.T) {
	world := newSupportWorld()
	world.repository.ticket = &support.Ticket{ID: uuid.New(), Email: "guest@example.com"}

	_, err := world.service.ReplyAsAgent(context.Background(), world.repository.ticket.ID, uuid.New(),
		strings.Repeat("r", support.MaxAgentReplyRunes+1))

	if !errors.Is(err, support.ErrInvalidTicket) {
		t.Fatalf("ReplyAsAgent() error = %v, want ErrInvalidTicket", err)
	}
	if world.tx.started {
		t.Fatal("an oversized reply opened a transaction")
	}
}

func TestChangeStatusRunsInsideATransaction(t *testing.T) {
	world := newSupportWorld()
	world.repository.ticket = &support.Ticket{ID: uuid.New(), Status: support.StatusOpen}

	ticket, err := world.service.ChangeStatus(context.Background(), world.repository.ticket.ID, uuid.New(), " resolved ")

	if err != nil {
		t.Fatalf("ChangeStatus() error = %v", err)
	}
	if ticket == nil || !world.tx.committed {
		t.Fatalf("ticket = %v, committed = %v", ticket, world.tx.committed)
	}
	if world.repository.transitionTarget != "resolved" {
		t.Fatalf("transitioned to %q, want the trimmed target", world.repository.transitionTarget)
	}
}

func TestListTicketsRefusesAnUnboundedPage(t *testing.T) {
	// The page size reaches a LIMIT. Without this a caller could ask for the
	// whole table in one request.
	world := newSupportWorld()
	for name, page := range map[string]struct{ page, limit int }{
		"page below one":     {0, 20},
		"limit below one":    {1, 0},
		"limit past the cap": {1, 101},
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := world.service.ListTickets(context.Background(), "open", page.page, page.limit); !errors.Is(err, support.ErrInvalidTicket) {
				t.Fatalf("ListTickets() error = %v, want ErrInvalidTicket", err)
			}
		})
	}
}

func TestListTicketsTranslatesThePageToAnOffset(t *testing.T) {
	world := newSupportWorld()

	if _, _, err := world.service.ListTickets(context.Background(), " open ", 3, 20); err != nil {
		t.Fatalf("ListTickets() error = %v", err)
	}
	if world.repository.listLimit != 20 || world.repository.listOffset != 40 {
		t.Fatalf("listed limit %d offset %d, want 20 and 40", world.repository.listLimit, world.repository.listOffset)
	}
	if world.repository.listStatus != "open" {
		t.Fatalf("filtered by %q, want the trimmed status", world.repository.listStatus)
	}
}

func TestGetTicketRefusesTheNilIdentifier(t *testing.T) {
	world := newSupportWorld()
	if _, _, err := world.service.GetTicket(context.Background(), uuid.Nil); !errors.Is(err, support.ErrInvalidTicket) {
		t.Fatalf("GetTicket() error = %v, want ErrInvalidTicket", err)
	}
}

// --- fakes ---

type supportWorld struct {
	service    *Service
	repository *fakeSupportRepository
	spam       *fakeSpamProtector
	customers  *fakeCustomerEmails
	tx         *fakeTransaction
	scheduler  *fakeScheduler
}

func newSupportWorld() *supportWorld {
	world := &supportWorld{
		repository: &fakeSupportRepository{},
		spam:       &fakeSpamProtector{},
		customers:  &fakeCustomerEmails{},
		tx:         &fakeTransaction{},
		scheduler:  &fakeScheduler{},
	}
	world.service = NewService(world.repository, world.spam, world.customers).
		WithAdminWorkflow(world.tx, world.scheduler)
	world.scheduler.tx = world.tx
	return world
}

type transactionKey struct{}

type fakeTransaction struct {
	started   bool
	committed bool
}

func (f *fakeTransaction) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	f.started = true
	if err := fn(context.WithValue(ctx, transactionKey{}, true)); err != nil {
		return err
	}
	f.committed = true
	return nil
}

type fakeSupportRepository struct {
	createdTicket    support.Ticket
	createdMessage   support.Message
	customerMessages int
	ticket           *support.Ticket
	transitionTarget string
	listStatus       string
	listLimit        int
	listOffset       int
}

func (f *fakeSupportRepository) Create(_ context.Context, ticket support.Ticket, message support.Message) (*support.Ticket, error) {
	f.createdTicket, f.createdMessage = ticket, message
	stored := ticket
	stored.ID = uuid.New()
	return &stored, nil
}

func (f *fakeSupportRepository) AddCustomerMessage(context.Context, uuid.UUID, uuid.UUID, string) error {
	f.customerMessages++
	return nil
}

func (f *fakeSupportRepository) List(_ context.Context, status string, limit, offset int) ([]support.Ticket, int64, error) {
	f.listStatus, f.listLimit, f.listOffset = status, limit, offset
	return nil, 0, nil
}

func (f *fakeSupportRepository) Get(context.Context, uuid.UUID) (*support.Ticket, []support.Message, error) {
	return f.ticket, nil, nil
}

func (f *fakeSupportRepository) AddAgentMessageAndSetPending(context.Context, uuid.UUID, uuid.UUID, string) (*support.Ticket, error) {
	return f.ticket, nil
}

func (f *fakeSupportRepository) TransitionStatus(_ context.Context, _ uuid.UUID, target string) (*support.Ticket, error) {
	f.transitionTarget = target
	return f.ticket, nil
}

type fakeSpamProtector struct {
	calls int
	err   error
}

func (f *fakeSpamProtector) Check(context.Context, string, string, string) error {
	f.calls++
	return f.err
}

type fakeCustomerEmails struct {
	email string
	asked uuid.UUID
	err   error
}

func (f *fakeCustomerEmails) EmailForCustomer(_ context.Context, id uuid.UUID) (string, error) {
	f.asked = id
	return f.email, f.err
}

type fakeScheduler struct {
	tx                *fakeTransaction
	template          string
	recipient         string
	insideTransaction bool
	err               error
}

func (f *fakeScheduler) ScheduleEmail(ctx context.Context, template, locale, recipient string, _ any) error {
	f.template, f.recipient = template, recipient
	f.insideTransaction = ctx.Value(transactionKey{}) == true
	_ = locale
	return f.err
}
