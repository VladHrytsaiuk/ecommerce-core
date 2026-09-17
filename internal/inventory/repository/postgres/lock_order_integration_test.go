//go:build integration

package postgres

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"
	postgresContainer "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// These establish the hazard the stock_items lock order exists to avoid, and
// measure the index the release path needs.
//
// The guard on the release query itself lives with the code that issues it:
// TestMarkPaidTakesStockRowsInTheOrderCheckoutReservesThem in
// platform/postgres/orderworkflow drives the real MarkPaid against a competing
// reservation. An earlier version of that guard lived here and asserted only
// that PostgreSQL sorts when asked to — which was never in doubt, and left the
// repository's own query unguarded.

// TestOppositeLockOrdersOnSharedStockDeadlock establishes the hazard rather
// than the fix. It takes two stock rows in opposite orders from two
// transactions and shows PostgreSQL aborts one of them. Every path that
// updates stock_items has to agree on an order, and this is what happens when
// two do not.
func TestOppositeLockOrdersOnSharedStockDeadlock(t *testing.T) {
	db, warehouseID := newStockFixture(t)
	first, second := seedStockItem(t, db, warehouseID), seedStockItem(t, db, warehouseID)
	// Sorted order, which is what checkout's ReserveBatch takes.
	ascending := []uuid.UUID{first, second}
	sort.Slice(ascending, func(i, j int) bool { return ascending[i].String() < ascending[j].String() })
	descending := []uuid.UUID{ascending[1], ascending[0]}

	errs := runConcurrently(t, db, warehouseID, ascending, descending)
	if !mentionsDeadlock(errs) {
		t.Fatalf("two transactions took the same rows in opposite orders without deadlocking: %v; "+
			"if PostgreSQL no longer detects this, the ordering requirement needs re-deriving", errs)
	}
}

// TestMatchingLockOrdersOnSharedStockDoNotDeadlock is the property the fix
// provides: with both sides taking the rows in the same sequence, the two
// transactions serialize instead of aborting.
func TestMatchingLockOrdersOnSharedStockDoNotDeadlock(t *testing.T) {
	db, warehouseID := newStockFixture(t)
	first, second := seedStockItem(t, db, warehouseID), seedStockItem(t, db, warehouseID)
	ascending := []uuid.UUID{first, second}
	sort.Slice(ascending, func(i, j int) bool { return ascending[i].String() < ascending[j].String() })

	errs := runConcurrently(t, db, warehouseID, ascending, ascending)
	for _, err := range errs {
		if err != nil {
			t.Fatalf("matching lock orders produced %v; they must serialize, not fail", err)
		}
	}
}

// TestReservationLookupByOrderUsesTheIndex measures the second half of the
// finding: the lookup was a sequential scan of the whole table.
func TestReservationLookupByOrderUsesTheIndex(t *testing.T) {
	db, warehouseID := newStockFixture(t)
	orderID := uuid.New()
	// One statement rather than five thousand round trips: the point is the
	// table's size, not how the rows got there.
	if err := db.Exec(`INSERT INTO inventory_reservations (idempotency_key, variant_id, warehouse_id, quantity, status, expires_at, order_id)
		SELECT gen_random_uuid(), gen_random_uuid(), ?, 1, 'active', CURRENT_TIMESTAMP + INTERVAL '1 hour', gen_random_uuid()
		FROM generate_series(1, 5000)`, warehouseID).Error; err != nil {
		t.Fatal(err)
	}
	seedReservationFor(t, db, warehouseID, orderID, uuid.New())
	if err := db.Exec("ANALYZE inventory_reservations").Error; err != nil {
		t.Fatal(err)
	}

	var plan []string
	if err := db.Raw(`EXPLAIN SELECT * FROM inventory_reservations WHERE order_id = ? FOR UPDATE`, orderID).Scan(&plan).Error; err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(plan, "\n")
	if strings.Contains(joined, "Seq Scan") {
		t.Fatalf("the lookup still scans the whole table:\n%s", joined)
	}
	if !strings.Contains(joined, "inventory_reservations_order_idx") {
		t.Fatalf("the order index is not used:\n%s", joined)
	}
}

// runConcurrently updates the two stock rows from two transactions, each in
// the order it is given.
//
// The barrier between the first and second statement is best-effort with a
// deadline, not a strict rendezvous. When the two orders match, the second
// transaction blocks inside its first statement and never reaches the
// barrier — a strict WaitGroup there would hang the test rather than observe
// the serialization it is meant to check.
func runConcurrently(t *testing.T, db *gorm.DB, warehouseID uuid.UUID, left, right []uuid.UUID) []error {
	t.Helper()
	errs := make([]error, 2)
	holding := make(chan struct{}, 2)
	var done sync.WaitGroup
	done.Add(2)

	run := func(slot int, order []uuid.UUID) {
		defer done.Done()
		errs[slot] = db.Transaction(func(tx *gorm.DB) error {
			if err := takeStock(tx, warehouseID, order[0]); err != nil {
				return err
			}
			holding <- struct{}{}
			waitForBothOrTimeout(holding)
			return takeStock(tx, warehouseID, order[1])
		})
	}
	go run(0, left)
	go run(1, right)
	done.Wait()
	return errs
}

// waitForBothOrTimeout returns once both transactions hold their first row, or
// after a short wait if only one ever will.
func waitForBothOrTimeout(holding chan struct{}) {
	deadline := time.After(2 * time.Second)
	for len(holding) < 2 {
		select {
		case <-deadline:
			return
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func takeStock(tx *gorm.DB, warehouseID, variantID uuid.UUID) error {
	return tx.Exec(`UPDATE stock_items SET quantity_reserved = quantity_reserved + 1, updated_at = CURRENT_TIMESTAMP
		WHERE variant_id = ? AND warehouse_id = ?`, variantID, warehouseID).Error
}

func mentionsDeadlock(errs []error) bool {
	for _, err := range errs {
		if err != nil && strings.Contains(strings.ToLower(err.Error()), "deadlock") {
			return true
		}
	}
	return false
}

func seedStockItem(t *testing.T, db *gorm.DB, warehouseID uuid.UUID) uuid.UUID {
	t.Helper()
	variantID := uuid.New()
	if err := db.Exec(`INSERT INTO stock_items (variant_id, warehouse_id, quantity_on_hand, quantity_reserved)
		VALUES (?, ?, 1000, 0)`, variantID, warehouseID).Error; err != nil {
		t.Fatal(err)
	}
	return variantID
}

func seedReservationFor(t *testing.T, db *gorm.DB, warehouseID, orderID, variantID uuid.UUID) {
	t.Helper()
	if err := db.Exec(`INSERT INTO inventory_reservations (id, idempotency_key, variant_id, warehouse_id, quantity, status, expires_at, order_id)
		VALUES (?, ?, ?, ?, 1, 'active', CURRENT_TIMESTAMP + INTERVAL '1 hour', ?)`,
		uuid.New(), uuid.New(), variantID, warehouseID, orderID).Error; err != nil {
		t.Fatal(err)
	}
}

func newStockFixture(t *testing.T) (*gorm.DB, uuid.UUID) {
	t.Helper()
	testcontainers.SkipIfProviderIsNotHealthy(t)
	ctx := context.Background()
	container, err := postgresContainer.Run(ctx, "postgres:16-alpine",
		postgresContainer.WithDatabase("stock_lock_test"),
		postgresContainer.WithUsername("stock"),
		postgresContainer.WithPassword("stock"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(60*time.Second)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })
	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE EXTENSION IF NOT EXISTS pgcrypto").Error; err != nil {
		t.Fatal(err)
	}
	_, file, _, _ := runtime.Caller(0)
	dir := filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "migrations", "modules", "inventory")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".up.sql") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		raw, readErr := os.ReadFile(filepath.Join(dir, name))
		if readErr != nil {
			t.Fatal(readErr)
		}
		if execErr := db.Exec(string(raw)).Error; execErr != nil {
			t.Fatal(fmt.Errorf("apply %s: %w", name, execErr))
		}
	}
	warehouseID := uuid.New()
	if err := db.Exec(`INSERT INTO warehouses (id, code, name) VALUES (?, 'main', 'Main')`, warehouseID).Error; err != nil {
		t.Fatal(err)
	}
	return db, warehouseID
}
