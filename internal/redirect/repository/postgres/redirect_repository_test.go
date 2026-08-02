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
	"github.com/VladHrytsaiuk/ecommerce-core/internal/redirect/domain"
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

type RedirectRepositoryTestSuite struct {
	suite.Suite
	pgContainer *postgres.PostgresContainer
	db          *gorm.DB
	repo        domain.RedirectRepository
	ctx         context.Context
}

func (s *RedirectRepositoryTestSuite) SetupSuite() {
	s.ctx = context.Background()

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

	s.repo = NewRedirectRepository(s.db)
}

func (s *RedirectRepositoryTestSuite) TearDownSuite() {
	if s.pgContainer != nil {
		s.Require().NoError(s.pgContainer.Terminate(s.ctx))
	}
}

func (s *RedirectRepositoryTestSuite) TearDownTest() {
	s.db.Exec(`TRUNCATE TABLE url_history, product, product_translation, category, category_translation, product_variation CASCADE`)
}

func (s *RedirectRepositoryTestSuite) TestSaveAndFindByOldSlug() {
	entityID := uuid.New()
	history := &domain.UrlHistory{
		ID:         uuid.New(),
		EntityType: "product",
		EntityID:   entityID,
		OldSlug:    "old-product-slug",
		CreatedAt:  time.Now(),
	}

	err := s.repo.SaveHistory(s.ctx, history)
	s.Require().NoError(err)

	found, err := s.repo.FindByOldSlug(s.ctx, "old-product-slug")
	s.Require().NoError(err)
	s.Require().NotNil(found)
	assert.Equal(s.T(), history.ID, found.ID)
	assert.Equal(s.T(), history.OldSlug, found.OldSlug)

	// Not found case
	notFound, err := s.repo.FindByOldSlug(s.ctx, "non-existent")
	s.Require().NoError(err)
	assert.Nil(s.T(), notFound)
}

func (s *RedirectRepositoryTestSuite) TestFindCurrentSlug_Product() {
	productID := uuid.New()
	s.Require().NoError(s.db.Exec("INSERT INTO product (id, updated_at) VALUES (?, NOW())", productID).Error)
	s.Require().NoError(s.db.Exec("INSERT INTO product_translation (product_id, language_code, slug, name) VALUES (?, 'uk', 'current-product', 'Product')", productID).Error)

	slug, err := s.repo.FindCurrentSlug(s.ctx, "product", productID, "uk")
	s.Require().NoError(err)
	assert.Equal(s.T(), "current-product", slug)
}

func (s *RedirectRepositoryTestSuite) TestFindCurrentSlug_Category() {
	categoryID := uuid.New()
	s.Require().NoError(s.db.Exec("INSERT INTO category (id, updated_at) VALUES (?, NOW())", categoryID).Error)
	s.Require().NoError(s.db.Exec("INSERT INTO category_translation (category_id, language_code, slug, name) VALUES (?, 'uk', 'current-category', 'Category')", categoryID).Error)

	slug, err := s.repo.FindCurrentSlug(s.ctx, "category", categoryID, "uk")
	s.Require().NoError(err)
	assert.Equal(s.T(), "current-category", slug)
}

func (s *RedirectRepositoryTestSuite) TestFindCurrentSlug_Variation() {
	productID := uuid.New()
	s.Require().NoError(s.db.Exec("INSERT INTO product (id, updated_at) VALUES (?, NOW())", productID).Error)

	variationID := uuid.New()
	s.Require().NoError(s.db.Exec("INSERT INTO product_variation (id, product_id, slug, updated_at) VALUES (?, ?, 'current-variation', NOW())", variationID, productID).Error)

	slug, err := s.repo.FindCurrentSlug(s.ctx, "product_variation", variationID, "uk")
	s.Require().NoError(err)
	assert.Equal(s.T(), "current-variation", slug)
}

func (s *RedirectRepositoryTestSuite) TestFindCurrentSlug_Unsupported() {
	slug, err := s.repo.FindCurrentSlug(s.ctx, "unknown", uuid.New(), "uk")
	s.Require().Error(err)
	assert.Contains(s.T(), err.Error(), "unsupported entity type")
	assert.Empty(s.T(), slug)
}

func TestRedirectRepositorySuite(t *testing.T) {
	suite.Run(t, new(RedirectRepositoryTestSuite))
}
