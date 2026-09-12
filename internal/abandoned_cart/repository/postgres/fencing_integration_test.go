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

	cart "github.com/VladHrytsaiuk/ecommerce-core/internal/abandoned_cart/domain"
)

// TestATakenOverClaimRefusesTheOldWorkersOutcome is the reason the campaign
// lease needed a token. Update and Requeue matched on status alone, so a
// worker whose five-minute lease had expired could still write its outcome
// over the claim another worker was running.
func TestATakenOverClaimRefusesTheOldWorkersOutcome(t *testing.T) {
	repository, db := newCampaignTestRepository(t)
	ctx := context.Background()
	seedDueCampaign(t, db)

	first, err := repository.ClaimDue(ctx, time.Now().UTC())
	if err != nil || first == nil {
		t.Fatalf("first ClaimDue() = (%v, %v)", first, err)
	}
	expireCampaignLease(t, db, first.ID)
	second, err := repository.ClaimDue(ctx, time.Now().UTC())
	if err != nil || second == nil {
		t.Fatalf("takeover ClaimDue() = (%v, %v)", second, err)
	}
	if second.LockToken == first.LockToken || second.LockToken == uuid.Nil {
		t.Fatalf("takeover token = %s, want a new one (previous %s)", second.LockToken, first.LockToken)
	}

	stale := *first
	stale.Status = "skipped"
	if err := repository.Update(ctx, &stale); err != nil {
		t.Fatalf("stale Update() error = %v", err)
	}
	if err := repository.Requeue(ctx, first.ID, first.LockToken); err != nil {
		t.Fatalf("stale Requeue() error = %v", err)
	}

	var status string
	if err := db.Raw(`SELECT status FROM abandoned_cart_campaigns WHERE id = ?`, first.ID).Row().Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "processing" {
		t.Fatalf("status = %q; a worker whose lease expired wrote over the live claim", status)
	}

	// The holder's own outcome still lands.
	live := *second
	live.Status = "scheduled"
	if err := repository.Update(ctx, &live); err != nil {
		t.Fatalf("live Update() error = %v", err)
	}
	if err := db.Raw(`SELECT status FROM abandoned_cart_campaigns WHERE id = ?`, first.ID).Row().Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "scheduled" {
		t.Fatalf("status = %q, want the live claim's outcome", status)
	}
}

func TestClaimDueLeavesNothingBehindOnAnEmptyQueue(t *testing.T) {
	repository, _ := newCampaignTestRepository(t)

	claimed, err := repository.ClaimDue(context.Background(), time.Now().UTC())
	if err != nil {
		t.Fatalf("ClaimDue() error = %v", err)
	}
	if claimed != nil {
		t.Fatalf("ClaimDue() = %+v, want nothing", claimed)
	}
}

func expireCampaignLease(t *testing.T, db *gorm.DB, campaignID uuid.UUID) {
	t.Helper()
	if err := db.Exec(`UPDATE abandoned_cart_campaigns SET locked_at = CURRENT_TIMESTAMP - INTERVAL '10 minutes' WHERE id = ?`, campaignID).Error; err != nil {
		t.Fatal(err)
	}
}

func seedDueCampaign(t *testing.T, db *gorm.DB) uuid.UUID {
	t.Helper()
	campaignID := uuid.New()
	if err := db.Exec(`INSERT INTO abandoned_cart_campaigns (id, cart_id, contact_email, step, status, due_at)
		VALUES (?, ?, 'buyer@example.com', 1, 'pending', CURRENT_TIMESTAMP - INTERVAL '1 minute')`,
		campaignID, uuid.New()).Error; err != nil {
		t.Fatal(err)
	}
	return campaignID
}

func newCampaignTestRepository(t *testing.T) (*Repository, *gorm.DB) {
	t.Helper()
	testcontainers.SkipIfProviderIsNotHealthy(t)
	ctx := context.Background()
	container, err := postgresContainer.Run(ctx, "postgres:16-alpine",
		postgresContainer.WithDatabase("abandoned_cart_test"),
		postgresContainer.WithUsername("cart"),
		postgresContainer.WithPassword("cart"),
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
	dir := filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "migrations", "modules", "abandoned_cart")
	for _, name := range []string{"000001_init_abandoned_cart.up.sql", "000002_fence_campaign_lease.up.sql"} {
		raw, readErr := os.ReadFile(filepath.Join(dir, name))
		if readErr != nil {
			t.Fatal(readErr)
		}
		if execErr := db.Exec(string(raw)).Error; execErr != nil {
			t.Fatalf("apply %s: %v", name, execErr)
		}
	}
	return NewRepository(db), db
}

var _ cart.Repository = (*Repository)(nil)
