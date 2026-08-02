//go:build legacy
// +build legacy

package postgres

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	platformDB "github.com/VladHrytsaiuk/ecommerce-core/internal/platform/db"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/wishlist/domain"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/gorm"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

type WishlistRepositoryTestSuite struct {
	suite.Suite
	pgContainer *postgres.PostgresContainer
	db          *gorm.DB
	repo        domain.WishlistRepository
	ctx         context.Context
}

func (s *WishlistRepositoryTestSuite) SetupSuite() {
	s.ctx = context.Background()
	logger.Init()

	dbName := "testdb"
	dbUser := "user"
	dbPassword := "password"

	container, err := postgres.Run(s.ctx,
		"postgres:16-alpine",
		postgres.WithDatabase(dbName),
		postgres.WithUsername(dbUser),
		postgres.WithPassword(dbPassword),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second)),
	)
	s.Require().NoError(err)
	s.pgContainer = container

	connStr, err := container.ConnectionString(s.ctx, "sslmode=disable")
	s.Require().NoError(err)

	_, b, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(b), "../../../..")
	migDir := fmt.Sprintf("file://%s/migrations", root)

	m, err := migrate.New(migDir, connStr)
	s.Require().NoError(err)
	err = m.Up()
	if err != nil && err != migrate.ErrNoChange {
		s.Require().NoError(err)
	}

	gormDB := platformDB.Connect(connStr)
	s.Require().NotNil(gormDB)
	s.db = gormDB

	s.repo = NewWishlistRepository(s.db, logger.Log)
}

func (s *WishlistRepositoryTestSuite) TearDownSuite() {
	if s.pgContainer != nil {
		s.Require().NoError(s.pgContainer.Terminate(s.ctx))
	}
}

func (s *WishlistRepositoryTestSuite) TearDownTest() {
	s.db.Exec(`TRUNCATE TABLE "user", wishlist, product_variation, product CASCADE`)
}

func (s *WishlistRepositoryTestSuite) createDummyVariation(variationID uuid.UUID) {
	productID := uuid.New()
	err := s.db.Exec("INSERT INTO product (id) VALUES (?)", productID).Error
	s.Require().NoError(err)
	err = s.db.Exec("INSERT INTO product_variation (id, product_id, price, is_active) VALUES (?, ?, 1000, true)", variationID, productID).Error
	s.Require().NoError(err)
}

func (s *WishlistRepositoryTestSuite) createDummyUser(userID uuid.UUID) {
	// Ensure a role exists
	s.db.Exec(`INSERT INTO role (id, name, description) VALUES (1, 'Customer', 'Default role') ON CONFLICT DO NOTHING`)
	err := s.db.Exec(`INSERT INTO "user" (id, email, password_hash, first_name, last_name, role_id) VALUES (?, ?, 'hash', 'John', 'Doe', 1)`, userID, userID.String()+"@test.com").Error
	s.Require().NoError(err)
}

func (s *WishlistRepositoryTestSuite) TestAddAndGetByUserID() {
	userID := uuid.New()
	variationID := uuid.New()
	s.createDummyUser(userID)
	s.createDummyVariation(variationID)

	item := &domain.WishlistItem{
		UserID:      &userID,
		VariationID: variationID,
	}

	err := s.repo.Add(s.ctx, item)
	s.Require().NoError(err)

	items, err := s.repo.GetByUserID(s.ctx, userID, "uk")
	s.Require().NoError(err)
	s.Require().Len(items, 1)
	assert.Equal(s.T(), variationID, items[0].ID)
}

func (s *WishlistRepositoryTestSuite) TestAddAndGetBySessionID() {
	sessionID := "sess-123"
	variationID := uuid.New()
	s.createDummyVariation(variationID)

	item := &domain.WishlistItem{
		SessionID:   &sessionID,
		VariationID: variationID,
	}

	err := s.repo.Add(s.ctx, item)
	s.Require().NoError(err)

	items, err := s.repo.GetBySessionID(s.ctx, sessionID, "uk")
	s.Require().NoError(err)
	s.Require().Len(items, 1)
	assert.Equal(s.T(), variationID, items[0].ID)
}

func (s *WishlistRepositoryTestSuite) TestRemove() {
	userID := uuid.New()
	variationID := uuid.New()
	s.createDummyUser(userID)
	s.createDummyVariation(variationID)

	item := &domain.WishlistItem{
		UserID:      &userID,
		VariationID: variationID,
	}

	s.Require().NoError(s.repo.Add(s.ctx, item))

	err := s.repo.Remove(s.ctx, &userID, nil, variationID)
	s.Require().NoError(err)

	items, err := s.repo.GetByUserID(s.ctx, userID, "uk")
	s.Require().NoError(err)
	assert.Empty(s.T(), items)
}

func (s *WishlistRepositoryTestSuite) TestRemove_NotFound() {
	userID := uuid.New()
	err := s.repo.Remove(s.ctx, &userID, nil, uuid.New())
	assert.ErrorIs(s.T(), err, domain.ErrItemNotInWishlist)
}

func (s *WishlistRepositoryTestSuite) TestSyncSessionToUser() {
	sessionID := "sess-sync-123"
	userID := uuid.New()
	variationID1 := uuid.New()
	variationID2 := uuid.New()

	s.createDummyUser(userID)
	s.createDummyVariation(variationID1)
	s.createDummyVariation(variationID2)

	// User already has variation1
	err := s.repo.Add(s.ctx, &domain.WishlistItem{
		UserID:      &userID,
		VariationID: variationID1,
	})
	s.Require().NoError(err)

	// Session has variation1 (duplicate) and variation2 (new)
	err = s.repo.Add(s.ctx, &domain.WishlistItem{
		SessionID:   &sessionID,
		VariationID: variationID1,
	})
	s.Require().NoError(err)

	err = s.repo.Add(s.ctx, &domain.WishlistItem{
		SessionID:   &sessionID,
		VariationID: variationID2,
	})
	s.Require().NoError(err)

	// Sync
	err = s.repo.SyncSessionToUser(s.ctx, sessionID, userID)
	s.Require().NoError(err)

	// Verify session is empty
	sessItems, err := s.repo.GetBySessionID(s.ctx, sessionID, "uk")
	s.Require().NoError(err)
	assert.Empty(s.T(), sessItems)

	// Verify user has both variations
	userItems, err := s.repo.GetByUserID(s.ctx, userID, "uk")
	s.Require().NoError(err)
	s.Require().Len(userItems, 2)
}

func (s *WishlistRepositoryTestSuite) TestDeleteExpiredAnonymous() {
	sessionID := "sess-exp"
	variationID := uuid.New()
	s.createDummyVariation(variationID)

	// Create item directly to mock created_at (since Add uses default CURRENT_TIMESTAMP)
	oldTime := time.Now().Add(-48 * time.Hour)
	err := s.db.Exec("INSERT INTO wishlist (session_id, variation_id, created_at) VALUES (?, ?, ?)", sessionID, variationID, oldTime).Error
	s.Require().NoError(err)

	err = s.repo.DeleteExpiredAnonymous(s.ctx, time.Now().Add(-24*time.Hour))
	s.Require().NoError(err)

	sessItems, err := s.repo.GetBySessionID(s.ctx, sessionID, "uk")
	s.Require().NoError(err)
	assert.Empty(s.T(), sessItems)
}

func (s *WishlistRepositoryTestSuite) TestVariationExists() {
	variationID := uuid.New()
	s.createDummyVariation(variationID)

	exists, err := s.repo.VariationExists(s.ctx, variationID)
	s.Require().NoError(err)
	assert.True(s.T(), exists)

	exists, err = s.repo.VariationExists(s.ctx, uuid.New())
	s.Require().NoError(err)
	assert.False(s.T(), exists)
}

func TestWishlistRepositorySuite(t *testing.T) {
	suite.Run(t, new(WishlistRepositoryTestSuite))
}
