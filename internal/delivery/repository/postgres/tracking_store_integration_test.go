//go:build integration

package postgres

import (
	"context"
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
)

// The tracker's whole correctness is this query. It used to take an arbitrary
// hundred rows with no ordering and no lease, from a set that included a
// terminal status, so a store's own delivery history eventually crowded out
// every shipment still in transit.

func TestATerminalDeliveryIsNeverClaimedAgain(t *testing.T) {
	store, db := newTrackingStore(t)
	now := time.Now().UTC()
	inTransit := seedDelivery(t, db, "in_transit", now.Add(-time.Hour))
	for _, terminal := range []string{"delivered", "failed", "cancelled"} {
		seedDelivery(t, db, terminal, now.Add(-time.Hour))
	}

	claimed, err := store.ClaimDue(context.Background(), 100, now, 5*time.Minute)
	if err != nil {
		t.Fatalf("ClaimDue() error = %v", err)
	}
	if len(claimed) != 1 || claimed[0].ID != inTransit {
		t.Fatalf("claimed %d deliveries; terminal ones are still in the working set", len(claimed))
	}
}

func TestADeliveryWithNoTrackingNumberIsNotClaimed(t *testing.T) {
	store, db := newTrackingStore(t)
	now := time.Now().UTC()
	orderID := seedOrder(t, db)
	if err := db.Exec(`INSERT INTO deliveries (id, order_id, provider, tracking_number, status, next_check_at)
		VALUES (?, ?, 'novaposhta', NULL, 'created', ?)`, uuid.New(), orderID, now.Add(-time.Hour)).Error; err != nil {
		t.Fatal(err)
	}

	claimed, err := store.ClaimDue(context.Background(), 100, now, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(claimed) != 0 {
		t.Fatal("claimed a delivery with nothing to ask the carrier about")
	}
}

func TestAClaimedDeliveryIsDeferredSoTheNextTickMovesOn(t *testing.T) {
	// Without this the same rows come back every tick and a backlog never
	// drains: the tracker polls its first hundred forever.
	store, db := newTrackingStore(t)
	now := time.Now().UTC()
	for range 3 {
		seedDelivery(t, db, "in_transit", now.Add(-time.Hour))
	}

	first, err := store.ClaimDue(context.Background(), 2, now, 5*time.Minute)
	if err != nil || len(first) != 2 {
		t.Fatalf("first claim = %d rows, %v", len(first), err)
	}
	second, err := store.ClaimDue(context.Background(), 2, now, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 1 {
		t.Fatalf("second claim = %d rows, want the one remaining; deferred rows came back", len(second))
	}
	if second[0].ID == first[0].ID || second[0].ID == first[1].ID {
		t.Fatal("a deferred delivery was handed out twice in one cycle")
	}
	// Once the interval passes they are due again.
	third, err := store.ClaimDue(context.Background(), 10, now.Add(6*time.Minute), 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(third) != 3 {
		t.Fatalf("after the interval %d of 3 deliveries were due", len(third))
	}
}

func TestTheOldestDueDeliveryIsClaimedFirst(t *testing.T) {
	// Arbitrary order is how some deliveries were polled forever and others
	// never, even below the batch size.
	//
	// Honest limit on what this proves: deliveries_due_idx is itself ordered by
	// next_check_at, so at this size the planner returns the same order with or
	// without the ORDER BY, and deleting the clause does not fail this test.
	// Reversing it does. The clause is what makes the order a property of the
	// query rather than of the plan the planner happened to pick.
	store, db := newTrackingStore(t)
	now := time.Now().UTC()
	// Inserted newest-due first, so physical order is the opposite of due
	// order. Seeding them in due order proves nothing: an unordered scan would
	// return the right row by accident.
	seedDelivery(t, db, "in_transit", now.Add(-time.Hour))
	seedDelivery(t, db, "in_transit", now.Add(-2*time.Hour))
	oldest := seedDelivery(t, db, "in_transit", now.Add(-3*time.Hour))

	claimed, err := store.ClaimDue(context.Background(), 1, now, 5*time.Minute)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("ClaimDue() = %d rows, %v", len(claimed), err)
	}
	if claimed[0].ID != oldest {
		t.Fatal("the batch did not start with the delivery waiting longest")
	}
}

func TestADeliveryNotYetDueIsLeftAlone(t *testing.T) {
	store, db := newTrackingStore(t)
	now := time.Now().UTC()
	seedDelivery(t, db, "in_transit", now.Add(time.Hour))

	claimed, err := store.ClaimDue(context.Background(), 100, now, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(claimed) != 0 {
		t.Fatal("claimed a delivery that is not due yet")
	}
}

func TestTheClaimCarriesWhatTheCarrierNeeds(t *testing.T) {
	store, db := newTrackingStore(t)
	now := time.Now().UTC()
	id := seedDelivery(t, db, "in_transit", now.Add(-time.Hour))

	claimed, err := store.ClaimDue(context.Background(), 100, now, 5*time.Minute)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("ClaimDue() = %d rows, %v", len(claimed), err)
	}
	got := claimed[0]
	if got.ID != id || got.Provider != "novaposhta" || got.TrackingNumber == "" || got.RecipientPhone != "+380000000000" {
		t.Fatalf("claimed = %+v; the carrier call needs the number and the recipient", got)
	}
}

func TestARejectedIntervalIsRefused(t *testing.T) {
	// A zero interval would defer nothing and reproduce the original bug.
	store, _ := newTrackingStore(t)
	if _, err := store.ClaimDue(context.Background(), 100, time.Now().UTC(), 0); err == nil {
		t.Fatal("ClaimDue() accepted an interval that defers nothing")
	}
}

func seedOrder(t *testing.T, db *gorm.DB) uuid.UUID {
	t.Helper()
	orderID := uuid.New()
	if err := db.Exec(`INSERT INTO orders (id, number, status, currency, subtotal_amount, total_amount)
		VALUES (?, ?, 'paid', 'EUR', 1000, 1000)`, orderID, "ORDER-"+orderID.String()[:12]).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO order_delivery_details (order_id, recipient_name, recipient_phone)
		VALUES (?, 'Recipient', '+380000000000')`, orderID).Error; err != nil {
		t.Fatal(err)
	}
	return orderID
}

func seedDelivery(t *testing.T, db *gorm.DB, status string, due time.Time) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if err := db.Exec(`INSERT INTO deliveries (id, order_id, provider, tracking_number, status, next_check_at)
		VALUES (?, ?, 'novaposhta', ?, ?, ?)`, id, seedOrder(t, db), "TN-"+id.String()[:12], status, due).Error; err != nil {
		t.Fatal(err)
	}
	return id
}

func newTrackingStore(t *testing.T) (*TrackingStore, *gorm.DB) {
	t.Helper()
	testcontainers.SkipIfProviderIsNotHealthy(t)
	ctx := context.Background()
	container, err := containerPostgres.Run(ctx, "postgres:16-alpine",
		containerPostgres.WithDatabase("delivery_test"),
		containerPostgres.WithUsername("delivery"),
		containerPostgres.WithPassword("delivery"),
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
	if err := migrateTrackingDir(filepath.Join(root, "migrations", "core"), dsn, "schema_migrations"); err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	return NewTrackingStore(db), db
}

func migrateTrackingDir(dir, dsn, table string) error {
	m, err := migrate.New("file://"+dir, trackingMigrationURL(dsn, table))
	if err != nil {
		return err
	}
	defer m.Close()
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}
	return nil
}

func trackingMigrationURL(dsn, table string) string {
	parsed, err := url.Parse(dsn)
	if err != nil {
		panic(err)
	}
	query := parsed.Query()
	query.Set("x-migrations-table", table)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
