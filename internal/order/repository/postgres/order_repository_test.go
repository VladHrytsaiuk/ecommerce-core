package postgres

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/order/domain"
	platformDB "github.com/VladHrytsaiuk/ecommerce-core/internal/platform/db"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"gorm.io/gorm"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

type OrderRepositoryTestSuite struct {
	suite.Suite
	pgContainer *postgres.PostgresContainer
	db          *gorm.DB
	repo        domain.OrderRepository
	ctx         context.Context
}

func (s *OrderRepositoryTestSuite) SetupSuite() {
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

	// Get project root path for migrations
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

	s.repo = NewOrderRepository(s.db, logger.Log)
}

func (s *OrderRepositoryTestSuite) TearDownSuite() {
	if s.pgContainer != nil {
		s.Require().NoError(s.pgContainer.Terminate(s.ctx))
	}
}

func (s *OrderRepositoryTestSuite) TearDownTest() {
	// Clean up tables
	s.db.Exec(`TRUNCATE TABLE delivery, order_item, "order", product_variation, product CASCADE`)
}

func (s *OrderRepositoryTestSuite) createDummyVariation(variationID uuid.UUID) {
	productID := uuid.New()
	err := s.db.Exec("INSERT INTO product (id) VALUES (?)", productID).Error
	s.Require().NoError(err)
	err = s.db.Exec("INSERT INTO product_variation (id, product_id, price) VALUES (?, ?, 1000)", variationID, productID).Error
	s.Require().NoError(err)
}

func (s *OrderRepositoryTestSuite) TestCreateAndFindByID() {
	orderID := uuid.New()

	order := &domain.Order{
		ID:         orderID,
		StatusID:   domain.StatusPendingPayment,
		FirstName:  "John",
		LastName:   "Doe",
		Email:      "john@example.com",
		Phone:      "+380123456789",
		TotalPrice: 1000,
	}

	variationID := uuid.New()
	s.createDummyVariation(variationID)

	items := []domain.OrderItem{
		{
			VariationID: variationID,
			Price:       1000,
			Quantity:    1,
			TotalPrice:  1000,
		},
	}

	delivery := &domain.Delivery{
		Provider:      "novaposhta",
		DeliveryType:  "warehouse",
		CityName:      "Kyiv",
		CityRef:       "ref1",
		WarehouseName: "W1",
		WarehouseRef:  "ref2",
	}

	err := s.repo.Create(s.ctx, order, items, delivery)
	s.Require().NoError(err)

	s.Require().NotZero(order.OrderNumber)

	foundOrder, err := s.repo.FindByID(s.ctx, orderID)
	s.Require().NoError(err)
	s.Require().NotNil(foundOrder)
	assert.Equal(s.T(), orderID, foundOrder.ID)
	assert.Equal(s.T(), "John", foundOrder.FirstName)
	assert.Equal(s.T(), 1, len(foundOrder.Items))
	assert.NotNil(s.T(), foundOrder.Delivery)
	assert.Equal(s.T(), "novaposhta", foundOrder.Delivery.Provider)
}

func (s *OrderRepositoryTestSuite) TestUpdateStatus() {
	orderID := uuid.New()
	order := &domain.Order{
		ID:         orderID,
		StatusID:   domain.StatusPendingPayment,
		FirstName:  "John",
		LastName:   "Doe",
		Email:      "john@example.com",
		Phone:      "+380123456789",
		TotalPrice: 1000,
	}

	variationID := uuid.New()
	s.createDummyVariation(variationID)

	items := []domain.OrderItem{
		{VariationID: variationID, Price: 1000, Quantity: 1, TotalPrice: 1000},
	}
	err := s.repo.Create(s.ctx, order, items, &domain.Delivery{})
	s.Require().NoError(err)

	err = s.repo.UpdateStatus(s.ctx, orderID, domain.StatusProcessing)
	s.Require().NoError(err)

	statusID, err := s.repo.GetOrderStatusByID(s.ctx, orderID)
	s.Require().NoError(err)
	assert.Equal(s.T(), domain.StatusProcessing, statusID)
}

func (s *OrderRepositoryTestSuite) TestFindByOrderNumber() {
	orderID := uuid.New()
	order := &domain.Order{
		ID:         orderID,
		StatusID:   domain.StatusPendingPayment,
		FirstName:  "Jane",
		LastName:   "Smith",
		TotalPrice: 500,
	}

	variationID := uuid.New()
	s.createDummyVariation(variationID)

	items := []domain.OrderItem{
		{VariationID: variationID, Price: 500, Quantity: 1, TotalPrice: 500},
	}

	err := s.repo.Create(s.ctx, order, items, &domain.Delivery{})
	s.Require().NoError(err)
	s.Require().NotZero(order.OrderNumber)

	foundOrder, err := s.repo.FindByOrderNumber(s.ctx, order.OrderNumber)
	s.Require().NoError(err)
	s.Require().NotNil(foundOrder)
	assert.Equal(s.T(), orderID, foundOrder.ID)
}

func TestOrderRepositorySuite(t *testing.T) {
	suite.Run(t, new(OrderRepositoryTestSuite))
}
