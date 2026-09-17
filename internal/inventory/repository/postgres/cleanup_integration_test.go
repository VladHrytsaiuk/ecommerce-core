//go:build integration

package postgres

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"
	postgresContainer "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// TestReleaseExpiredUnattachedReportsADatabaseFailure covers the difference
// between the two reasons a single release can fail. A row another replica
// holds must be skipped; a broken statement must not be reported as a
// successful sweep, because "released 0" is also what a quiet night looks like
// and the reservations go on holding stock either way.
func TestReleaseExpiredUnattachedReportsADatabaseFailure(t *testing.T) {
	repository, db := newInventoryTestRepository(t)
	seedExpiredReservation(t, db)

	// stock_items is what releaseOne updates; removing it makes that statement
	// fail with a real database error rather than a missing row.
	if err := db.Exec(`DROP TABLE stock_items`).Error; err != nil {
		t.Fatal(err)
	}

	released, err := repository.ReleaseExpiredUnattached(context.Background(), time.Now().UTC(), 100)
	if err == nil {
		t.Fatal("ReleaseExpiredUnattached() error = nil; a failing sweep reported success and the breakage stayed invisible")
	}
	if released != 0 {
		t.Fatalf("released = %d, want 0", released)
	}
}

// TestReleaseExpiredUnattachedSkipsAReservationHeldElsewhere is the other half:
// the skip that must survive. A row locked by another replica reads back as
// missing, and the sweep has to carry on with the rest.
func TestReleaseExpiredUnattachedSkipsAReservationHeldElsewhere(t *testing.T) {
	repository, db := newInventoryTestRepository(t)
	held := seedExpiredReservation(t, db)
	free := seedExpiredReservation(t, db)

	// An open transaction elsewhere holding the row is what SKIP LOCKED steps
	// over; the repository sees it as not found.
	other := db.Begin()
	t.Cleanup(func() { _ = other.Rollback() })
	if err := other.Exec(`SELECT id FROM inventory_reservations WHERE id = ? FOR UPDATE`, held).Error; err != nil {
		t.Fatal(err)
	}

	released, err := repository.ReleaseExpiredUnattached(context.Background(), time.Now().UTC(), 100)
	if err != nil {
		t.Fatalf("ReleaseExpiredUnattached() error = %v, want the locked row to be skipped", err)
	}
	if released != 1 {
		t.Fatalf("released = %d, want 1: the unlocked reservation must still be settled", released)
	}
	var status string
	if err := db.Raw(`SELECT status FROM inventory_reservations WHERE id = ?`, free).Row().Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "expired" {
		t.Fatalf("free reservation status = %q, want expired", status)
	}
}

func seedExpiredReservation(t *testing.T, db *gorm.DB) uuid.UUID {
	t.Helper()
	warehouseID, variantID, reservationID := testWarehouseID(t, db), uuid.New(), uuid.New()
	if err := db.Exec(`INSERT INTO stock_items (variant_id, warehouse_id, quantity_on_hand, quantity_reserved) VALUES (?, ?, 10, 1)`,
		variantID, warehouseID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO inventory_reservations (id, idempotency_key, variant_id, warehouse_id, quantity, status, expires_at)
		VALUES (?, ?, ?, ?, 1, 'active', CURRENT_TIMESTAMP - INTERVAL '1 hour')`,
		reservationID, uuid.New(), variantID, warehouseID).Error; err != nil {
		t.Fatal(err)
	}
	return reservationID
}

// testWarehouseID returns the single warehouse of this fixture, creating it on
// first use so each seeded reservation can share it.
func testWarehouseID(t *testing.T, db *gorm.DB) uuid.UUID {
	t.Helper()
	var existing uuid.UUID
	if err := db.Raw(`SELECT id FROM warehouses WHERE code = 'main'`).Row().Scan(&existing); err == nil && existing != uuid.Nil {
		return existing
	}
	warehouseID := uuid.New()
	if err := db.Exec(`INSERT INTO warehouses (id, code, name) VALUES (?, 'main', 'Main')`, warehouseID).Error; err != nil {
		t.Fatal(err)
	}
	return warehouseID
}

func newInventoryTestRepository(t *testing.T) (*Repository, *gorm.DB) {
	t.Helper()
	testcontainers.SkipIfProviderIsNotHealthy(t)
	ctx := context.Background()
	container, err := postgresContainer.Run(ctx, "postgres:16-alpine",
		postgresContainer.WithDatabase("inventory_test"),
		postgresContainer.WithUsername("inventory"),
		postgresContainer.WithPassword("inventory"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(30*time.Second)))
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
	root := filepath.Join(filepath.Dir(file), "..", "..", "..", "..")
	for _, migration := range []string{
		"migrations/modules/inventory/000001_init_inventory.up.sql",
		"migrations/modules/inventory/000002_quarantine_failed_reservation_release.up.sql",
	} {
		raw, err := os.ReadFile(filepath.Join(root, migration))
		if err != nil {
			t.Fatal(err)
		}
		if err := db.Exec(string(raw)).Error; err != nil {
			t.Fatal(err)
		}
	}
	return NewRepository(db), db
}
