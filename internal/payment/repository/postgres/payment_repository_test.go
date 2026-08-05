//go:build legacy && ignore
// +build legacy,ignore

package postgres

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/payment/domain"
	platformDB "github.com/VladHrytsaiuk/ecommerce-core/internal/platform/db"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
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

type PaymentRepositoryTestSuite struct {
	suite.Suite
	pgContainer *postgres.PostgresContainer
	db          *gorm.DB
	repo        domain.PaymentRepository
	ctx         context.Context
}

func (s *PaymentRepositoryTestSuite) SetupSuite() {
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

	s.repo = NewPaymentRepository(s.db, logger.Log)
}

func (s *PaymentRepositoryTestSuite) TearDownSuite() {
	if s.pgContainer != nil {
		s.Require().NoError(s.pgContainer.Terminate(s.ctx))
	}
}

func (s *PaymentRepositoryTestSuite) TearDownTest() {
	s.db.Exec(`TRUNCATE TABLE payment, "order" CASCADE`)
}

func (s *PaymentRepositoryTestSuite) createDummyOrder(orderID uuid.UUID) {
	err := s.db.Exec(`INSERT INTO "order" (id, status_id) VALUES (?, 1)`, orderID).Error
	s.Require().NoError(err)
}

func (s *PaymentRepositoryTestSuite) TestCreateAndFind() {
	orderID := uuid.New()
	s.createDummyOrder(orderID)

	paymentID := uuid.New()
	payment := &domain.Payment{
		ID:       paymentID,
		OrderID:  orderID,
		Provider: "liqpay",
		Amount:   1000,
		Currency: "UAH",
		Status:   "pending",
	}

	err := s.repo.Create(s.ctx, payment)
	s.Require().NoError(err)

	found, err := s.repo.FindByOrderID(s.ctx, orderID)
	s.Require().NoError(err)
	s.Require().NotNil(found)
	assert.Equal(s.T(), paymentID, found.ID)
	assert.Equal(s.T(), "liqpay", found.Provider)
	assert.Equal(s.T(), "pending", found.Status)
}

func (s *PaymentRepositoryTestSuite) TestFindByOrderID_NotFound() {
	found, err := s.repo.FindByOrderID(s.ctx, uuid.New())
	assert.ErrorIs(s.T(), err, domain.ErrPaymentNotFound)
	assert.Nil(s.T(), found)
}

func (s *PaymentRepositoryTestSuite) TestUpdateStatus() {
	orderID := uuid.New()
	s.createDummyOrder(orderID)

	paymentID := uuid.New()
	payment := &domain.Payment{
		ID:       paymentID,
		OrderID:  orderID,
		Provider: "liqpay",
		Amount:   1000,
		Currency: "UAH",
		Status:   "pending",
	}

	err := s.repo.Create(s.ctx, payment)
	s.Require().NoError(err)

	err = s.repo.UpdateStatus(s.ctx, paymentID, "success", "txn123", "")
	s.Require().NoError(err)

	found, err := s.repo.FindByOrderID(s.ctx, orderID)
	s.Require().NoError(err)
	assert.Equal(s.T(), "success", found.Status)
	assert.Equal(s.T(), "txn123", found.TransactionID)
}

func (s *PaymentRepositoryTestSuite) TestUpdateStatus_NotFound() {
	err := s.repo.UpdateStatus(s.ctx, uuid.New(), "success", "", "")
	assert.ErrorIs(s.T(), err, domain.ErrPaymentNotFound)
}

func TestPaymentRepositorySuite(t *testing.T) {
	suite.Run(t, new(PaymentRepositoryTestSuite))
}
