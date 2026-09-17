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

	video "github.com/VladHrytsaiuk/ecommerce-core/internal/video/domain"
)

func TestAttachRefusesAnAssetThatIsNotReady(t *testing.T) {
	repository, _ := newVideoTestRepository(t)
	ctx := context.Background()
	productID := uuid.New()

	for _, status := range []video.AssetStatus{video.AssetDraft, video.AssetUploading, video.AssetProcessing, video.AssetFailed} {
		assetID := seedAsset(t, repository, status)
		_, err := repository.Attach(ctx, video.Placement{ProductID: productID, AssetID: assetID, Role: video.RolePreview, Position: 0, IsVisible: true})
		// A placement pointing at an unfinished encoding would sit invisible on
		// the storefront until somebody noticed the gap.
		if !errors.Is(err, video.ErrAssetNotPlayable) {
			t.Fatalf("Attach(%s) error = %v, want ErrAssetNotPlayable", status, err)
		}
	}
}

func TestAttachRejectsASecondPlacementInTheSameSlot(t *testing.T) {
	repository, _ := newVideoTestRepository(t)
	ctx := context.Background()
	productID := uuid.New()

	if _, err := repository.Attach(ctx, video.Placement{ProductID: productID, AssetID: seedAsset(t, repository, video.AssetReady), Role: video.RolePreview, Position: 0, IsVisible: true}); err != nil {
		t.Fatalf("first Attach() error = %v", err)
	}
	// (product_id, role, position) is unique. Two administrators racing for the
	// same slot must get a conflict, not a driver error.
	_, err := repository.Attach(ctx, video.Placement{ProductID: productID, AssetID: seedAsset(t, repository, video.AssetReady), Role: video.RolePreview, Position: 0, IsVisible: true})
	if !errors.Is(err, video.ErrPositionTaken) {
		t.Fatalf("second Attach() error = %v, want ErrPositionTaken", err)
	}
}

func TestAttachRejectsTheSameAssetTwiceOnOneProduct(t *testing.T) {
	repository, _ := newVideoTestRepository(t)
	ctx := context.Background()
	productID, assetID := uuid.New(), seedAsset(t, repository, video.AssetReady)

	if _, err := repository.Attach(ctx, video.Placement{ProductID: productID, AssetID: assetID, Role: video.RolePreview, Position: 0, IsVisible: true}); err != nil {
		t.Fatalf("first Attach() error = %v", err)
	}
	_, err := repository.Attach(ctx, video.Placement{ProductID: productID, AssetID: assetID, Role: video.RoleHowToUse, Position: 1, IsVisible: true})
	if !errors.Is(err, video.ErrPlacementDuplicate) {
		t.Fatalf("duplicate Attach() error = %v, want ErrPlacementDuplicate", err)
	}
}

func TestDetachIsScopedToItsProduct(t *testing.T) {
	repository, _ := newVideoTestRepository(t)
	ctx := context.Background()
	owner, stranger := uuid.New(), uuid.New()

	placed, err := repository.Attach(ctx, video.Placement{ProductID: owner, AssetID: seedAsset(t, repository, video.AssetReady), Role: video.RolePreview, IsVisible: true})
	if err != nil {
		t.Fatalf("Attach() error = %v", err)
	}
	// Knowing a placement ID must not be enough to remove it from under a
	// different product.
	if err := repository.Detach(ctx, stranger, placed.ID); !errors.Is(err, video.ErrPlacementNotFound) {
		t.Fatalf("cross-product Detach() error = %v, want ErrPlacementNotFound", err)
	}
	if err := repository.Detach(ctx, owner, placed.ID); err != nil {
		t.Fatalf("Detach() error = %v", err)
	}
}

func TestHiddenAndUnreadyPlacementsNeverReachTheStorefront(t *testing.T) {
	repository, db := newVideoTestRepository(t)
	ctx := context.Background()
	productID := uuid.New()

	visible, err := repository.Attach(ctx, video.Placement{ProductID: productID, AssetID: seedAsset(t, repository, video.AssetReady), Role: video.RolePreview, Position: 0, IsVisible: true})
	if err != nil {
		t.Fatalf("Attach() error = %v", err)
	}
	hidden, err := repository.Attach(ctx, video.Placement{ProductID: productID, AssetID: seedAsset(t, repository, video.AssetReady), Role: video.RolePreview, Position: 1, IsVisible: false})
	if err != nil {
		t.Fatalf("Attach() error = %v", err)
	}
	// Move one asset out of ready behind the placement's back, as the cleanup
	// reconciler or a failed re-encode would.
	degraded, err := repository.Attach(ctx, video.Placement{ProductID: productID, AssetID: seedAsset(t, repository, video.AssetReady), Role: video.RolePreview, Position: 2, IsVisible: true})
	if err != nil {
		t.Fatalf("Attach() error = %v", err)
	}
	if err := db.Exec(`UPDATE video_assets SET status = 'failed' WHERE id = ?`, degraded.AssetID).Error; err != nil {
		t.Fatal(err)
	}

	public, err := repository.ListReadyProductVideos(ctx, productID)
	if err != nil {
		t.Fatalf("ListReadyProductVideos() error = %v", err)
	}
	if len(public) != 1 || public[0].ID != visible.ID {
		t.Fatalf("storefront returned %d placements, want only the visible ready one", len(public))
	}

	// The administrative view is deliberately the opposite: it shows the rows
	// somebody has to fix.
	all, err := repository.ListProductVideos(ctx, productID)
	if err != nil {
		t.Fatalf("ListProductVideos() error = %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("admin view returned %d placements, want all 3 including %s", len(all), hidden.ID)
	}
}

func TestUpdatePlacementReordersAndRejectsAnOccupiedSlot(t *testing.T) {
	repository, _ := newVideoTestRepository(t)
	ctx := context.Background()
	productID := uuid.New()

	first, err := repository.Attach(ctx, video.Placement{ProductID: productID, AssetID: seedAsset(t, repository, video.AssetReady), Role: video.RolePreview, Position: 0, IsVisible: true})
	if err != nil {
		t.Fatalf("Attach() error = %v", err)
	}
	second, err := repository.Attach(ctx, video.Placement{ProductID: productID, AssetID: seedAsset(t, repository, video.AssetReady), Role: video.RolePreview, Position: 1, IsVisible: true})
	if err != nil {
		t.Fatalf("Attach() error = %v", err)
	}

	updated, err := repository.UpdatePlacement(ctx, productID, second.ID, 5, false)
	if err != nil {
		t.Fatalf("UpdatePlacement() error = %v", err)
	}
	if updated.Position != 5 || updated.IsVisible {
		t.Fatalf("UpdatePlacement() = %+v, want position 5 and hidden", updated)
	}
	if _, err := repository.UpdatePlacement(ctx, productID, second.ID, 0, true); !errors.Is(err, video.ErrPositionTaken) {
		t.Fatalf("UpdatePlacement() into %s's slot error = %v, want ErrPositionTaken", first.ID, err)
	}
}

func seedAsset(t *testing.T, repository *Repository, status video.AssetStatus) uuid.UUID {
	t.Helper()
	id := uuid.New()
	external := "stream-" + id.String()
	if err := repository.db.Exec(
		`INSERT INTO video_assets (id, provider, external_id, status, duration_seconds, poster_url)
		 VALUES (?, 'cloudflare', ?, ?, 12, 'https://video.example.test/poster.jpg')`,
		id, external, string(status)).Error; err != nil {
		t.Fatal(err)
	}
	return id
}

func newVideoTestRepository(t *testing.T) (*Repository, *gorm.DB) {
	t.Helper()
	testcontainers.SkipIfProviderIsNotHealthy(t)
	ctx := context.Background()
	container, err := containerPostgres.Run(ctx, "postgres:16-alpine",
		containerPostgres.WithDatabase("video_test"),
		containerPostgres.WithUsername("video"),
		containerPostgres.WithPassword("video"),
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
		{filepath.Join(root, "migrations", "modules", "video"), "schema_migrations_module_video"},
	} {
		if err := migrateVideoDir(step.dir, dsn, step.table); err != nil {
			t.Fatal(err)
		}
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	return NewRepository(db), db
}

func migrateVideoDir(dir, dsn, table string) error {
	m, err := migrate.New("file://"+dir, videoMigrationURL(dsn, table))
	if err != nil {
		return err
	}
	defer m.Close()
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}
	return nil
}

func videoMigrationURL(dsn, table string) string {
	parsed, err := url.Parse(dsn)
	if err != nil {
		panic(err)
	}
	query := parsed.Query()
	query.Set("x-migrations-table", table)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
