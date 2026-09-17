//go:build integration

package postgres

import (
	"context"
	"errors"
	"net/url"
	"path/filepath"
	"runtime"
	"testing"
	"time"

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
	returns "github.com/VladHrytsaiuk/ecommerce-core/internal/returns/domain"
)

// A return request is three tables written together. Nothing below the database
// can show that the items and the history land attached to the request they
// belong to, which is exactly the mistake that went unnoticed in support.

func TestAReturnRequestStoresItsItemsAndHistoryAttachedToIt(t *testing.T) {
	repository, db := newReturnsTestRepository(t)
	ctx := context.Background()
	request := newRequest(t, uuid.New(), uuid.New())

	if err := repository.Create(ctx, request); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	var items, history int64
	if err := db.Raw(`SELECT COUNT(*) FROM return_items WHERE return_request_id = ?`, request.ID).Scan(&items).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Raw(`SELECT COUNT(*) FROM return_status_history WHERE return_request_id = ?`, request.ID).Scan(&history).Error; err != nil {
		t.Fatal(err)
	}
	if items != 2 || history != 1 {
		t.Fatalf("attached %d items and %d history rows, want 2 and 1", items, history)
	}
	var orphans int64
	if err := db.Raw(`
		SELECT (SELECT COUNT(*) FROM return_items i
		         WHERE NOT EXISTS (SELECT 1 FROM return_requests r WHERE r.id = i.return_request_id))
		     + (SELECT COUNT(*) FROM return_status_history h
		         WHERE NOT EXISTS (SELECT 1 FROM return_requests r WHERE r.id = h.return_request_id))`).Scan(&orphans).Error; err != nil {
		t.Fatal(err)
	}
	if orphans != 0 {
		t.Fatalf("%d rows reference no return request", orphans)
	}
}

func TestTheDatabaseRefusesAChildRowWithNoParent(t *testing.T) {
	// This is the guard the returns module did not have. Without it a wrong
	// identifier is stored silently and the row is invisible from then on.
	repository, db := newReturnsTestRepository(t)
	request := newRequest(t, uuid.New(), uuid.New())
	if err := repository.Create(context.Background(), request); err != nil {
		t.Fatal(err)
	}

	if err := db.Exec(`INSERT INTO return_items (id, return_request_id, variant_id, quantity, condition)
		VALUES (?, ?, ?, 1, 'unopened')`, uuid.New(), uuid.New(), uuid.New()).Error; err == nil {
		t.Fatal("the database accepted a return item belonging to no request")
	}
	if err := db.Exec(`INSERT INTO return_status_history (id, return_request_id, status, actor_type, actor_id)
		VALUES (?, ?, 'new', 'customer', ?)`, uuid.New(), uuid.New(), uuid.New()).Error; err == nil {
		t.Fatal("the database accepted a history row belonging to no request")
	}
	if err := db.Exec(`INSERT INTO return_restock_operations (return_item_id, return_request_id)
		VALUES (?, ?)`, uuid.New(), request.ID).Error; err == nil {
		t.Fatal("the database accepted a restock operation for an item that does not exist")
	}
}

func TestAReturnRequestIsReadBackWholeAndInOrder(t *testing.T) {
	repository, _ := newReturnsTestRepository(t)
	ctx := context.Background()
	orderID, customerID := uuid.New(), uuid.New()
	request := newRequest(t, orderID, customerID)
	if err := repository.Create(ctx, request); err != nil {
		t.Fatal(err)
	}

	loaded, err := repository.Get(ctx, request.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if loaded.OrderID != orderID || loaded.CustomerID != customerID || loaded.Status != returns.ReturnStatusNew {
		t.Fatalf("loaded = %+v", loaded)
	}
	if len(loaded.Items) != 2 || len(loaded.History) != 1 {
		t.Fatalf("loaded %d items and %d history rows", len(loaded.Items), len(loaded.History))
	}
	// The restock adapter writes its idempotency row from this field; a zero
	// value here would key every operation on nothing.
	for _, item := range loaded.Items {
		if item.ReturnRequestID != request.ID {
			t.Fatalf("item %s carries request %s, want %s", item.ID, item.ReturnRequestID, request.ID)
		}
	}
	if _, err := repository.Get(ctx, uuid.New()); !errors.Is(err, returns.ErrReturnRequestNotFound) {
		t.Fatalf("Get(missing) error = %v, want ErrReturnRequestNotFound", err)
	}
}

func TestUpdateRefusesToWriteHistoryForARequestThatIsGone(t *testing.T) {
	// The history row is the audit trail. Writing one for a request that no
	// longer exists would leave a transition nothing can be traced back to.
	repository, db := newReturnsTestRepository(t)
	ctx := context.Background()
	request := newRequest(t, uuid.New(), uuid.New())
	if err := repository.Create(ctx, request); err != nil {
		t.Fatal(err)
	}

	admin := uuid.New()
	missing := *request
	missing.ID = uuid.New()
	missing.Status = returns.ReturnStatusApproved
	err := repository.Update(ctx, &missing, returns.ReturnStatusHistory{
		ID: uuid.New(), ReturnRequestID: missing.ID, Status: returns.ReturnStatusApproved,
		ActorType: returns.ActorTypeAdmin, ActorID: &admin, CreatedAt: time.Now().UTC(),
	})
	if !errors.Is(err, returns.ErrReturnRequestNotFound) {
		t.Fatalf("Update() error = %v, want ErrReturnRequestNotFound", err)
	}
	var written int64
	if err := db.Raw(`SELECT COUNT(*) FROM return_status_history WHERE return_request_id = ?`, missing.ID).Scan(&written).Error; err != nil {
		t.Fatal(err)
	}
	if written != 0 {
		t.Fatal("a history row was written for a request that does not exist")
	}
}

func TestUpdateMovesTheStatusAndAppendsOneHistoryRow(t *testing.T) {
	repository, _ := newReturnsTestRepository(t)
	ctx := context.Background()
	request := newRequest(t, uuid.New(), uuid.New())
	if err := repository.Create(ctx, request); err != nil {
		t.Fatal(err)
	}

	admin := uuid.New()
	request.Status = returns.ReturnStatusApproved
	request.UpdatedAt = time.Now().UTC()
	if err := repository.Update(ctx, request, returns.ReturnStatusHistory{
		ID: uuid.New(), ReturnRequestID: request.ID, Status: returns.ReturnStatusApproved,
		ActorType: returns.ActorTypeAdmin, ActorID: &admin, Reason: "within policy", CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	loaded, err := repository.Get(ctx, request.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != returns.ReturnStatusApproved || len(loaded.History) != 2 {
		t.Fatalf("status = %q with %d history rows", loaded.Status, len(loaded.History))
	}
	if loaded.History[1].Reason != "within policy" {
		t.Fatalf("history = %+v, want the newest transition last", loaded.History)
	}
}

func TestLockingAReturnRequestRequiresTheCallersTransaction(t *testing.T) {
	// A FOR UPDATE lock taken outside the caller's transaction is released the
	// moment the statement ends, which is worse than no lock: it reads as one.
	repository, db := newReturnsTestRepository(t)
	ctx := context.Background()
	request := newRequest(t, uuid.New(), uuid.New())
	if err := repository.Create(ctx, request); err != nil {
		t.Fatal(err)
	}
	admin := uuid.New()
	request.Status = returns.ReturnStatusReceived
	request.UpdatedAt = time.Now().UTC()
	if err := repository.Update(ctx, request, returns.ReturnStatusHistory{
		ID: uuid.New(), ReturnRequestID: request.ID, Status: returns.ReturnStatusReceived,
		ActorType: returns.ActorTypeAdmin, ActorID: &admin, CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := repository.FindReceivedByOrderForUpdate(ctx, request.OrderID); err == nil {
		t.Fatal("FindReceivedByOrderForUpdate() succeeded outside a transaction")
	}
	if err := transaction.Within(ctx, db, func(tx *gorm.DB) error {
		found, err := repository.FindReceivedByOrderForUpdate(transaction.WithContext(ctx, tx), request.OrderID)
		if err != nil {
			return err
		}
		if found.ID != request.ID {
			t.Fatalf("found %s, want %s", found.ID, request.ID)
		}
		return nil
	}); err != nil {
		t.Fatalf("FindReceivedByOrderForUpdate() in a transaction error = %v", err)
	}
	// Only a received return is claimable; an order with none must say so.
	if err := transaction.Within(ctx, db, func(tx *gorm.DB) error {
		_, err := repository.FindReceivedByOrderForUpdate(transaction.WithContext(ctx, tx), uuid.New())
		return err
	}); !errors.Is(err, returns.ErrReturnRequestNotFound) {
		t.Fatalf("FindReceivedByOrderForUpdate(other order) error = %v, want ErrReturnRequestNotFound", err)
	}
}

func TestCreateRefusesAnIncompleteRequest(t *testing.T) {
	repository, db := newReturnsTestRepository(t)
	ctx := context.Background()
	base := newRequest(t, uuid.New(), uuid.New())

	for name, mutate := range map[string]func(*returns.ReturnRequest){
		"no id":       func(r *returns.ReturnRequest) { r.ID = uuid.Nil },
		"no order":    func(r *returns.ReturnRequest) { r.OrderID = uuid.Nil },
		"no customer": func(r *returns.ReturnRequest) { r.CustomerID = uuid.Nil },
		"no items":    func(r *returns.ReturnRequest) { r.Items = nil },
		"no history":  func(r *returns.ReturnRequest) { r.History = nil },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := *base
			candidate.Items = append([]returns.ReturnItem(nil), base.Items...)
			candidate.History = append([]returns.ReturnStatusHistory(nil), base.History...)
			mutate(&candidate)
			if err := repository.Create(ctx, &candidate); err == nil {
				t.Fatal("Create() accepted an incomplete request")
			}
		})
	}
	var stored int64
	if err := db.Raw(`SELECT COUNT(*) FROM return_requests`).Scan(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored != 0 {
		t.Fatalf("%d requests were stored from rejected input", stored)
	}
}

func newRequest(t *testing.T, orderID, customerID uuid.UUID) *returns.ReturnRequest {
	t.Helper()
	actor := customerID
	request, err := returns.NewReturnRequest(orderID, customerID, returns.RefundModeFull,
		[]returns.ReturnItem{
			{VariantID: uuid.New(), Quantity: 1, Condition: returns.ItemConditionUnopened},
			{VariantID: uuid.New(), Quantity: 2, Condition: returns.ItemConditionOpened, Reason: "too small"},
		},
		returns.Actor{Type: returns.ActorTypeCustomer, ID: &actor}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func newReturnsTestRepository(t *testing.T) (*Repository, *gorm.DB) {
	t.Helper()
	testcontainers.SkipIfProviderIsNotHealthy(t)
	ctx := context.Background()
	container, err := containerPostgres.Run(ctx, "postgres:16-alpine",
		containerPostgres.WithDatabase("returns_test"),
		containerPostgres.WithUsername("returns"),
		containerPostgres.WithPassword("returns"),
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
		{filepath.Join(root, "migrations", "modules", "returns"), "schema_migrations_module_returns"},
	} {
		if err := migrateReturnsDir(step.dir, dsn, step.table); err != nil {
			t.Fatal(err)
		}
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	return NewRepository(db), db
}

func migrateReturnsDir(dir, dsn, table string) error {
	m, err := migrate.New("file://"+dir, returnsMigrationURL(dsn, table))
	if err != nil {
		return err
	}
	defer m.Close()
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}
	return nil
}

func returnsMigrationURL(dsn, table string) string {
	parsed, err := url.Parse(dsn)
	if err != nil {
		panic(err)
	}
	query := parsed.Query()
	query.Set("x-migrations-table", table)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
