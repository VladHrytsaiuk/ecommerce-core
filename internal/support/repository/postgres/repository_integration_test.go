//go:build integration

package postgres

import (
	"context"
	"errors"
	"net/url"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"
	containerPostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/postgres/transaction"
	support "github.com/VladHrytsaiuk/ecommerce-core/internal/support/domain"
)

// The only thing standing between one customer and another's support thread is
// the customer_id predicate in AddCustomerMessage's lookup. That is SQL, so
// nothing below the database proves it holds.

func TestACustomerCannotPostIntoAnotherCustomersTicket(t *testing.T) {
	repository, db := newSupportTestRepository(t)
	ctx := context.Background()
	owner, stranger := uuid.New(), uuid.New()
	ticketID := seedTicket(t, repository, &owner, support.StatusOpen)

	err := repository.AddCustomerMessage(ctx, ticketID, stranger, "let me read this thread")

	if !errors.Is(err, support.ErrTicketNotFound) {
		t.Fatalf("AddCustomerMessage() error = %v, want ErrTicketNotFound", err)
	}
	// Reported as missing rather than forbidden: confirming the ticket exists
	// would already leak that somebody else has one with that ID.
	var messages int64
	if err := db.Raw(`SELECT COUNT(*) FROM support_messages WHERE ticket_id = ? AND sender_id = ?`, ticketID, stranger).Scan(&messages).Error; err != nil {
		t.Fatal(err)
	}
	if messages != 0 {
		t.Fatal("a message from another customer was written into the ticket")
	}
}

func TestTheOwnerCanPostIntoTheirOwnTicket(t *testing.T) {
	repository, db := newSupportTestRepository(t)
	ctx := context.Background()
	owner := uuid.New()
	ticketID := seedTicket(t, repository, &owner, support.StatusOpen)

	if err := repository.AddCustomerMessage(ctx, ticketID, owner, "any update?"); err != nil {
		t.Fatalf("AddCustomerMessage() error = %v", err)
	}
	var body string
	if err := db.Raw(`SELECT body FROM support_messages WHERE ticket_id = ? AND sender_type = 'customer' ORDER BY created_at DESC, id DESC LIMIT 1`, ticketID).Scan(&body).Error; err != nil {
		t.Fatal(err)
	}
	if body != "any update?" {
		t.Fatalf("stored body = %q", body)
	}
}

func TestNobodyCanPostIntoAGuestTicket(t *testing.T) {
	// A guest ticket has a NULL customer_id, and `customer_id = ?` never
	// matches NULL. That is what keeps a guessed ticket ID from becoming an
	// authenticated way into someone else's thread.
	repository, _ := newSupportTestRepository(t)
	ctx := context.Background()
	ticketID := seedTicket(t, repository, nil, support.StatusOpen)

	if err := repository.AddCustomerMessage(ctx, ticketID, uuid.New(), "hello"); !errors.Is(err, support.ErrTicketNotFound) {
		t.Fatalf("AddCustomerMessage() error = %v, want ErrTicketNotFound", err)
	}
}

func TestAClosedTicketRefusesFurtherCustomerMessages(t *testing.T) {
	repository, _ := newSupportTestRepository(t)
	ctx := context.Background()
	owner := uuid.New()
	ticketID := seedTicket(t, repository, &owner, support.StatusClosed)

	if err := repository.AddCustomerMessage(ctx, ticketID, owner, "reopening this"); !errors.Is(err, support.ErrMessageForbidden) {
		t.Fatalf("AddCustomerMessage() error = %v, want ErrMessageForbidden", err)
	}
}

func TestAnAgentReplyAndItsStatusChangeAreOneUnit(t *testing.T) {
	repository, db := newSupportTestRepository(t)
	ctx := context.Background()
	owner, agent := uuid.New(), uuid.New()
	ticketID := seedTicket(t, repository, &owner, support.StatusOpen)

	// The caller owns the transaction: the reply, the status change and the
	// notification the service schedules all commit together or not at all.
	err := transaction.Within(ctx, db, func(*gorm.DB) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	var updated *support.Ticket
	if err := transaction.Within(ctx, db, func(tx *gorm.DB) error {
		var innerErr error
		updated, innerErr = repository.AddAgentMessageAndSetPending(transaction.WithContext(ctx, tx), ticketID, agent, "looking into it")
		return innerErr
	}); err != nil {
		t.Fatalf("AddAgentMessageAndSetPending() error = %v", err)
	}
	if updated.Status != support.StatusPendingCustomer {
		t.Fatalf("status = %q, want %q", updated.Status, support.StatusPendingCustomer)
	}

	// A failure after the reply leaves neither the message nor the status.
	before := countMessages(t, db, ticketID)
	rollback := errors.New("notification scheduling failed")
	if err := transaction.Within(ctx, db, func(tx *gorm.DB) error {
		if _, innerErr := repository.AddAgentMessageAndSetPending(transaction.WithContext(ctx, tx), ticketID, agent, "second reply"); innerErr != nil {
			return innerErr
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("transaction error = %v, want the rollback", err)
	}
	if after := countMessages(t, db, ticketID); after != before {
		t.Fatalf("messages = %d after a rolled-back reply, want %d", after, before)
	}
}

func TestAnAgentRoutePassesOverAMissingTicket(t *testing.T) {
	repository, db := newSupportTestRepository(t)
	ctx := context.Background()
	missing := uuid.New()

	err := transaction.Within(ctx, db, func(tx *gorm.DB) error {
		_, innerErr := repository.AddAgentMessageAndSetPending(transaction.WithContext(ctx, tx), missing, uuid.New(), "hello")
		return innerErr
	})
	if !errors.Is(err, support.ErrTicketNotFound) {
		t.Fatalf("AddAgentMessageAndSetPending() error = %v, want ErrTicketNotFound", err)
	}
	err = transaction.Within(ctx, db, func(tx *gorm.DB) error {
		_, innerErr := repository.TransitionStatus(transaction.WithContext(ctx, tx), missing, support.StatusClosed)
		return innerErr
	})
	if !errors.Is(err, support.ErrTicketNotFound) {
		t.Fatalf("TransitionStatus() error = %v, want ErrTicketNotFound", err)
	}
}

func TestTheListingIsBoundedAndOrderedMostRecentlyUpdatedFirst(t *testing.T) {
	repository, _ := newSupportTestRepository(t)
	ctx := context.Background()
	owner := uuid.New()
	for range 3 {
		seedTicket(t, repository, &owner, support.StatusOpen)
	}
	seedTicket(t, repository, &owner, support.StatusClosed)

	all, total, err := repository.List(ctx, "", 10, 0)
	if err != nil || total != 4 || len(all) != 4 {
		t.Fatalf("List() = (%d rows, total %d, %v)", len(all), total, err)
	}
	open, total, err := repository.List(ctx, support.StatusOpen, 10, 0)
	if err != nil || total != 3 || len(open) != 3 {
		t.Fatalf("List(open) = (%d rows, total %d, %v)", len(open), total, err)
	}
	// The total counts the whole filtered set, not the page, or the admin UI
	// would stop paginating after the first page.
	page, total, err := repository.List(ctx, "", 2, 0)
	if err != nil || total != 4 || len(page) != 2 {
		t.Fatalf("List(page) = (%d rows, total %d, %v)", len(page), total, err)
	}
	for _, limit := range []int{0, -1, 101} {
		if _, _, err := repository.List(ctx, "", limit, 0); !errors.Is(err, support.ErrInvalidTicket) {
			t.Fatalf("List(limit=%d) error = %v, want ErrInvalidTicket", limit, err)
		}
	}
	if _, _, err := repository.List(ctx, "", 10, -1); !errors.Is(err, support.ErrInvalidTicket) {
		t.Fatal("List() accepted a negative offset")
	}
}

func TestTheOpeningMessageIsAttachedToTheTicketItOpened(t *testing.T) {
	// The customer's description of the problem is the first message on the
	// ticket. Storing it with a nil ticket_id detached it from the thread: the
	// agent opened the ticket and saw a subject line with nothing under it.
	repository, db := newSupportTestRepository(t)
	owner := uuid.New()
	ticketID := seedTicket(t, repository, &owner, support.StatusNew)

	var attached int64
	if err := db.Raw(`SELECT COUNT(*) FROM support_messages WHERE ticket_id = ? AND body = 'first'`, ticketID).Scan(&attached).Error; err != nil {
		t.Fatal(err)
	}
	if attached != 1 {
		t.Fatalf("the opening message is attached to %d rows of this ticket, want 1", attached)
	}
	var orphaned int64
	if err := db.Raw(`SELECT COUNT(*) FROM support_messages m WHERE NOT EXISTS (SELECT 1 FROM support_tickets t WHERE t.id = m.ticket_id)`).Scan(&orphaned).Error; err != nil {
		t.Fatal(err)
	}
	if orphaned != 0 {
		t.Fatalf("%d support messages reference no ticket at all", orphaned)
	}
}

func TestGetReturnsTheThreadInTheOrderItWasWritten(t *testing.T) {
	repository, db := newSupportTestRepository(t)
	ctx := context.Background()
	owner, agent := uuid.New(), uuid.New()
	ticketID := seedTicket(t, repository, &owner, support.StatusOpen)
	if err := repository.AddCustomerMessage(ctx, ticketID, owner, "second"); err != nil {
		t.Fatal(err)
	}
	if err := transaction.Within(ctx, db, func(tx *gorm.DB) error {
		_, innerErr := repository.AddAgentMessageAndSetPending(transaction.WithContext(ctx, tx), ticketID, agent, "third")
		return innerErr
	}); err != nil {
		t.Fatal(err)
	}

	ticket, messages, err := repository.Get(ctx, ticketID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if ticket.ID != ticketID {
		t.Fatalf("Get() returned ticket %s", ticket.ID)
	}
	if len(messages) != 3 {
		t.Fatalf("thread has %d messages, want 3", len(messages))
	}
	for index, want := range []string{"first", "second", "third"} {
		if messages[index].Body != want {
			t.Fatalf("message %d = %q, want %q", index, messages[index].Body, want)
		}
	}
	if _, _, err := repository.Get(ctx, uuid.New()); !errors.Is(err, support.ErrTicketNotFound) {
		t.Fatalf("Get(missing) error = %v, want ErrTicketNotFound", err)
	}
}

func seedTicket(t *testing.T, repository *Repository, customerID *uuid.UUID, status string) uuid.UUID {
	t.Helper()
	created, err := repository.Create(context.Background(),
		support.Ticket{CustomerID: customerID, Email: "customer@example.test", Subject: "Question", Status: support.StatusNew},
		support.Message{SenderType: "customer", SenderID: customerID, Body: "first"})
	if err != nil {
		t.Fatal(err)
	}
	if status != support.StatusNew {
		if err := transaction.Within(context.Background(), repository.db, func(tx *gorm.DB) error {
			return tx.Exec(`UPDATE support_tickets SET status = ? WHERE id = ?`, status, created.ID).Error
		}); err != nil {
			t.Fatal(err)
		}
	}
	return created.ID
}

func countMessages(t *testing.T, db *gorm.DB, ticketID uuid.UUID) int64 {
	t.Helper()
	var count int64
	if err := db.Raw(`SELECT COUNT(*) FROM support_messages WHERE ticket_id = ?`, ticketID).Scan(&count).Error; err != nil {
		t.Fatal(err)
	}
	return count
}

func newSupportTestRepository(t *testing.T) (*Repository, *gorm.DB) {
	t.Helper()
	testcontainers.SkipIfProviderIsNotHealthy(t)
	ctx := context.Background()
	container, err := containerPostgres.Run(ctx, "postgres:16-alpine",
		containerPostgres.WithDatabase("support_test"),
		containerPostgres.WithUsername("support"),
		containerPostgres.WithPassword("support"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(file), "..", "..", "..", "..")
	for _, step := range []struct{ dir, table string }{
		{filepath.Join(root, "migrations", "core"), "schema_migrations"},
		{filepath.Join(root, "migrations", "modules", "support"), "schema_migrations_module_support"},
	} {
		if err := migrateSupportDir(step.dir, dsn, step.table); err != nil {
			t.Fatal(err)
		}
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	return NewRepository(db), db
}

func migrateSupportDir(dir, dsn, table string) error {
	m, err := migrate.New("file://"+dir, supportMigrationURL(dsn, table))
	if err != nil {
		return err
	}
	defer m.Close()
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}
	return nil
}

func supportMigrationURL(dsn, table string) string {
	parsed, err := url.Parse(dsn)
	if err != nil {
		panic(err)
	}
	query := parsed.Query()
	query.Set("x-migrations-table", table)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
