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
	"github.com/VladHrytsaiuk/ecommerce-core/internal/sitemap/domain"
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

type SitemapRepositoryTestSuite struct {
	suite.Suite
	pgContainer *postgres.PostgresContainer
	db          *gorm.DB
	repo        domain.SitemapRepository
	ctx         context.Context
}

func (s *SitemapRepositoryTestSuite) SetupSuite() {
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

	s.repo = NewSitemapRepository(s.db)
}

func (s *SitemapRepositoryTestSuite) TearDownSuite() {
	if s.pgContainer != nil {
		s.Require().NoError(s.pgContainer.Terminate(s.ctx))
	}
}

func (s *SitemapRepositoryTestSuite) TearDownTest() {
	s.db.Exec(`TRUNCATE TABLE product, product_translation, category, category_translation, brand, documents CASCADE`)
}

func (s *SitemapRepositoryTestSuite) TestGetActiveProductSlugs() {
	productID := uuid.New()
	s.Require().NoError(s.db.Exec("INSERT INTO product (id, is_active, updated_at) VALUES (?, true, NOW())", productID).Error)
	s.Require().NoError(s.db.Exec("INSERT INTO product_translation (product_id, language_code, slug, name) VALUES (?, 'uk', 'test-product', 'Test Product')", productID).Error)

	// Inactive product should be excluded
	inactiveID := uuid.New()
	s.Require().NoError(s.db.Exec("INSERT INTO product (id, is_active, updated_at) VALUES (?, false, NOW())", inactiveID).Error)
	s.Require().NoError(s.db.Exec("INSERT INTO product_translation (product_id, language_code, slug, name) VALUES (?, 'uk', 'inactive-product', 'Inactive Product')", inactiveID).Error)

	slugs, err := s.repo.GetActiveProductSlugs(s.ctx)
	s.Require().NoError(err)
	s.Require().Len(slugs, 1)
	assert.Equal(s.T(), "test-product", slugs[0].Slug)
	assert.Equal(s.T(), "uk", slugs[0].LanguageCode)
}

func (s *SitemapRepositoryTestSuite) TestGetCategorySlugs() {
	categoryID := uuid.New()
	s.Require().NoError(s.db.Exec("INSERT INTO category (id, updated_at) VALUES (?, NOW())", categoryID).Error)
	s.Require().NoError(s.db.Exec("INSERT INTO category_translation (category_id, language_code, slug, name) VALUES (?, 'uk', 'test-category', 'Test Category')", categoryID).Error)

	slugs, err := s.repo.GetCategorySlugs(s.ctx)
	s.Require().NoError(err)
	s.Require().Len(slugs, 1)
	assert.Equal(s.T(), "test-category", slugs[0].Slug)
	assert.Equal(s.T(), "uk", slugs[0].LanguageCode)
}

func (s *SitemapRepositoryTestSuite) TestGetBrandSlugs() {
	brandID := uuid.New()
	s.Require().NoError(s.db.Exec("INSERT INTO brand (id, slug, name, updated_at) VALUES (?, 'test-brand', 'Test Brand', NOW())", brandID).Error)

	slugs, err := s.repo.GetBrandSlugs(s.ctx)
	s.Require().NoError(err)
	s.Require().Len(slugs, 1)
	assert.Equal(s.T(), "test-brand", slugs[0].Slug)
	// Brand doesn't have language code in translations in the query, it just returns slug
	assert.Equal(s.T(), "", slugs[0].LanguageCode)
}

func (s *SitemapRepositoryTestSuite) TestGetDocumentSlugs() {
	docID := uuid.New()
	s.Require().NoError(s.db.Exec("INSERT INTO documents (id, slug, title, updated_at) VALUES (?, 'test-doc', '{\"uk\": \"Test Doc\"}', NOW())", docID).Error)

	slugs, err := s.repo.GetDocumentSlugs(s.ctx)
	s.Require().NoError(err)
	s.Require().Len(slugs, 1)
	assert.Equal(s.T(), "test-doc", slugs[0].Slug)
	assert.Equal(s.T(), "", slugs[0].LanguageCode)
}

func TestSitemapRepositorySuite(t *testing.T) {
	suite.Run(t, new(SitemapRepositoryTestSuite))
}
