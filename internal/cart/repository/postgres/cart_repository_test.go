//go:build integration

package postgres

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/db"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"gorm.io/gorm"
	"go.uber.org/zap"
)

type noopLogger struct{}

func (n *noopLogger) Debug(msg string, fields ...zap.Field)       {}
func (n *noopLogger) Info(msg string, fields ...zap.Field)        {}
func (n *noopLogger) Warn(msg string, fields ...zap.Field)        {}
func (n *noopLogger) Error(msg string, fields ...zap.Field)       {}
func (n *noopLogger) Fatal(msg string, fields ...zap.Field)       {}
func (n *noopLogger) Debugf(template string, args ...interface{}) {}
func (n *noopLogger) Infof(template string, args ...interface{})  {}
func (n *noopLogger) Warnf(template string, args ...interface{})  {}
func (n *noopLogger) Errorf(template string, args ...interface{}) {}
func (n *noopLogger) Fatalf(template string, args ...interface{}) {}
func (n *noopLogger) Debugw(msg string, kvs ...interface{})       {}
func (n *noopLogger) Infow(msg string, kvs ...interface{})        {}
func (n *noopLogger) Warnw(msg string, kvs ...interface{})        {}
func (n *noopLogger) Errorw(msg string, kvs ...interface{})       {}
func (n *noopLogger) Fatalw(msg string, kvs ...interface{})       {}
func (n *noopLogger) With(fields ...zap.Field) logger.Logger      { return n }
func (n *noopLogger) Sync() error                                 { return nil }

func setupTestData(t *testing.T, gormDB *gorm.DB) (uuid.UUID, uuid.UUID, uuid.UUID) {
	// Create required tables and data for cart tests
	brandID := uuid.New()
	catID := uuid.New()
	prodID := uuid.New()
	variationID := uuid.New()

	require.NoError(t, gormDB.Exec(`INSERT INTO brand (id, name) VALUES (?, 'B')`, brandID).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO category (id, sort_order) VALUES (?, 0)`, catID).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO product (id, brand_id, category_id, is_active, slug) VALUES (?, ?, ?, true, ?)`, prodID, brandID, catID, "prod-1").Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO product_variation (id, product_id, sku, price, stock, is_active) VALUES (?, ?, ?, ?, ?, ?)`, variationID, prodID, "SKU1", 1000, 10, true).Error)
	
	// translations
	require.NoError(t, gormDB.Exec(`INSERT INTO product_translation (product_id, language_code, name) VALUES (?, 'uk', 'Name')`, prodID).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO category_translation (category_id, language_code, name) VALUES (?, 'uk', 'Cat')`, catID).Error)

	return brandID, catID, variationID
}

func TestCartRepository_AddAndGetCart(t *testing.T) {
	gormDB, cleanup := db.SetupTestDB(t)
	defer cleanup()

	repo := NewCartRepository(gormDB, &noopLogger{})
	ctx := context.Background()

	_, _, variationID := setupTestData(t, gormDB)
	userID := uuid.New()

	// Add item
	err := repo.AddItem(ctx, &userID, nil, variationID, 2)
	require.NoError(t, err)

	// Get by User ID
	cart, variations, err := repo.GetByUserID(ctx, userID, "uk")
	require.NoError(t, err)
	assert.NotNil(t, cart)
	assert.Len(t, cart.Items, 1)
	assert.Equal(t, 2, cart.Items[0].Quantity)
	assert.Len(t, variations, 1)
	assert.Equal(t, variationID, variations[0].ID)

	// Update Quantity
	err = repo.UpdateQuantity(ctx, &userID, nil, variationID, 5)
	require.NoError(t, err)

	cart, _, err = repo.GetByUserID(ctx, userID, "uk")
	require.NoError(t, err)
	assert.Equal(t, 5, cart.Items[0].Quantity)

	// Remove Item
	err = repo.RemoveItem(ctx, &userID, nil, variationID)
	require.NoError(t, err)

	cart, _, err = repo.GetByUserID(ctx, userID, "uk")
	require.NoError(t, err)
	assert.Len(t, cart.Items, 0)
}

func TestCartRepository_SyncSessionToUser(t *testing.T) {
	gormDB, cleanup := db.SetupTestDB(t)
	defer cleanup()

	repo := NewCartRepository(gormDB, &noopLogger{})
	ctx := context.Background()

	_, _, variationID := setupTestData(t, gormDB)
	userID := uuid.New()
	sessionID := "sess-123"

	// Add to session
	err := repo.AddItem(ctx, nil, &sessionID, variationID, 2)
	require.NoError(t, err)

	// Sync
	err = repo.SyncSessionToUser(ctx, sessionID, userID)
	require.NoError(t, err)

	// Check user cart
	cart, _, err := repo.GetByUserID(ctx, userID, "uk")
	require.NoError(t, err)
	assert.Len(t, cart.Items, 1)
	assert.Equal(t, 2, cart.Items[0].Quantity)
}
