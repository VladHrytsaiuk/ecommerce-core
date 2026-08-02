//go:build legacy
// +build legacy

package service

import (
	"context"
	"testing"
	"time"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/cart/domain"
	discountDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/discount/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	productDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/product/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/pagination"
	shipmentDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/shipment/domain"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// --- Mock Cart Repository ---

type mockCartRepo struct {
	getByUserIDFn            func(ctx context.Context, userID uuid.UUID, lang string) (*domain.Cart, []productDomain.ProductVariation, error)
	getBySessionIDFn         func(ctx context.Context, sessionID string, lang string) (*domain.Cart, []productDomain.ProductVariation, error)
	addItemFn                func(ctx context.Context, userID *uuid.UUID, sessionID *string, variationID uuid.UUID, quantity int) error
	updateQuantityFn         func(ctx context.Context, userID *uuid.UUID, sessionID *string, variationID uuid.UUID, quantity int) error
	removeItemFn             func(ctx context.Context, userID *uuid.UUID, sessionID *string, variationID uuid.UUID) error
	syncSessionToUserFn      func(ctx context.Context, sessionID string, userID uuid.UUID) error
	deleteExpiredAnonymousFn func(ctx context.Context, olderThan time.Time) error
	variationExistsFn        func(ctx context.Context, variationID uuid.UUID) (bool, error)
	updatePromoCodeFn        func(ctx context.Context, userID *uuid.UUID, sessionID *string, promoCodeID *uuid.UUID) error
}

func (m *mockCartRepo) GetByUserID(ctx context.Context, userID uuid.UUID, lang string) (*domain.Cart, []productDomain.ProductVariation, error) {
	return m.getByUserIDFn(ctx, userID, lang)
}
func (m *mockCartRepo) GetBySessionID(ctx context.Context, sessionID string, lang string) (*domain.Cart, []productDomain.ProductVariation, error) {
	return m.getBySessionIDFn(ctx, sessionID, lang)
}
func (m *mockCartRepo) AddItem(ctx context.Context, userID *uuid.UUID, sessionID *string, variationID uuid.UUID, quantity int) error {
	return m.addItemFn(ctx, userID, sessionID, variationID, quantity)
}
func (m *mockCartRepo) UpdateQuantity(ctx context.Context, userID *uuid.UUID, sessionID *string, variationID uuid.UUID, quantity int) error {
	return m.updateQuantityFn(ctx, userID, sessionID, variationID, quantity)
}
func (m *mockCartRepo) RemoveItem(ctx context.Context, userID *uuid.UUID, sessionID *string, variationID uuid.UUID) error {
	return m.removeItemFn(ctx, userID, sessionID, variationID)
}
func (m *mockCartRepo) SyncSessionToUser(ctx context.Context, sessionID string, userID uuid.UUID) error {
	return m.syncSessionToUserFn(ctx, sessionID, userID)
}
func (m *mockCartRepo) DeleteExpiredAnonymous(ctx context.Context, olderThan time.Time) error {
	return m.deleteExpiredAnonymousFn(ctx, olderThan)
}
func (m *mockCartRepo) VariationExists(ctx context.Context, variationID uuid.UUID) (bool, error) {
	return m.variationExistsFn(ctx, variationID)
}
func (m *mockCartRepo) UpdatePromoCode(ctx context.Context, userID *uuid.UUID, sessionID *string, promoCodeID *uuid.UUID) error {
	if m.updatePromoCodeFn != nil {
		return m.updatePromoCodeFn(ctx, userID, sessionID, promoCodeID)
	}
	return nil
}

// --- Mock Shipment Service ---

type mockShipmentService struct {
	threshold int
	err       error
}

func (m *mockShipmentService) GetAreas(ctx context.Context, provider string) ([]shipmentDomain.Area, error) {
	return nil, nil
}
func (m *mockShipmentService) GetCities(ctx context.Context, provider string, areaRef string) ([]shipmentDomain.City, error) {
	return nil, nil
}
func (m *mockShipmentService) GetWarehouses(ctx context.Context, provider string, cityRef string, warehouseType string) ([]shipmentDomain.Warehouse, error) {
	return nil, nil
}
func (m *mockShipmentService) GetFreeShippingThreshold(ctx context.Context) (int, error) {
	return m.threshold, m.err
}
func (m *mockShipmentService) GetMinimumOrderAmount(ctx context.Context) (int, error) {
	return 0, nil
}
func (m *mockShipmentService) UpdateShippingRule(ctx context.Context, provider string, minOrderAmount int) error {
	return nil
}

// --- Mock Logger ---

type mockLogger struct{}

func (m *mockLogger) Debug(msg string, fields ...zap.Field)       {}
func (m *mockLogger) Info(msg string, fields ...zap.Field)        {}
func (m *mockLogger) Warn(msg string, fields ...zap.Field)        {}
func (m *mockLogger) Error(msg string, fields ...zap.Field)       {}
func (m *mockLogger) Fatal(msg string, fields ...zap.Field)       {}
func (m *mockLogger) Debugf(template string, args ...interface{}) {}
func (m *mockLogger) Infof(template string, args ...interface{})  {}
func (m *mockLogger) Warnf(template string, args ...interface{})  {}
func (m *mockLogger) Errorf(template string, args ...interface{}) {}
func (m *mockLogger) Fatalf(template string, args ...interface{}) {}
func (m *mockLogger) Debugw(msg string, kvs ...interface{})       {}
func (m *mockLogger) Infow(msg string, kvs ...interface{})        {}
func (m *mockLogger) Warnw(msg string, kvs ...interface{})        {}
func (m *mockLogger) Errorw(msg string, kvs ...interface{})       {}
func (m *mockLogger) Fatalw(msg string, kvs ...interface{})       {}
func (m *mockLogger) With(fields ...zap.Field) logger.Logger      { return m }
func (m *mockLogger) Sync() error                                 { return nil }

// --- Mock Promo Service ---

type mockPromoService struct {
	createPromoFn           func(ctx context.Context, p *discountDomain.PromoCode) (*discountDomain.PromoCode, error)
	getPromoByIDFn          func(ctx context.Context, id uuid.UUID) (*discountDomain.PromoCode, error)
	getPromoByCodeFn        func(ctx context.Context, code string) (*discountDomain.PromoCode, error)
	updatePromoFn           func(ctx context.Context, id uuid.UUID, p *discountDomain.PromoCode) (*discountDomain.PromoCode, error)
	deletePromoFn           func(ctx context.Context, id uuid.UUID) error
	listPromosFn            func(ctx context.Context, pgn pagination.Params) ([]discountDomain.PromoCode, pagination.Metadata, error)
	calculateCartDiscountFn func(ctx context.Context, promoID uuid.UUID, items []discountDomain.PromoItemInfo) (*discountDomain.PromoCalculationResult, error)
	validatePromoLimitsFn   func(ctx context.Context, promoID uuid.UUID, userID *uuid.UUID, email, phone *string) error
}

func (m *mockPromoService) CreatePromo(ctx context.Context, p *discountDomain.PromoCode) (*discountDomain.PromoCode, error) {
	if m.createPromoFn != nil {
		return m.createPromoFn(ctx, p)
	}
	return nil, nil
}
func (m *mockPromoService) GetPromoByID(ctx context.Context, id uuid.UUID) (*discountDomain.PromoCode, error) {
	if m.getPromoByIDFn != nil {
		return m.getPromoByIDFn(ctx, id)
	}
	return nil, nil
}
func (m *mockPromoService) GetPromoByCode(ctx context.Context, code string) (*discountDomain.PromoCode, error) {
	if m.getPromoByCodeFn != nil {
		return m.getPromoByCodeFn(ctx, code)
	}
	return nil, nil
}
func (m *mockPromoService) UpdatePromo(ctx context.Context, id uuid.UUID, p *discountDomain.PromoCode) (*discountDomain.PromoCode, error) {
	if m.updatePromoFn != nil {
		return m.updatePromoFn(ctx, id, p)
	}
	return nil, nil
}
func (m *mockPromoService) DeletePromo(ctx context.Context, id uuid.UUID) error {
	if m.deletePromoFn != nil {
		return m.deletePromoFn(ctx, id)
	}
	return nil
}
func (m *mockPromoService) ListPromos(ctx context.Context, pgn pagination.Params) ([]discountDomain.PromoCode, pagination.Metadata, error) {
	if m.listPromosFn != nil {
		return m.listPromosFn(ctx, pgn)
	}
	return nil, pagination.Metadata{}, nil
}
func (m *mockPromoService) CalculateCartDiscount(ctx context.Context, promoID uuid.UUID, items []discountDomain.PromoItemInfo) (*discountDomain.PromoCalculationResult, error) {
	if m.calculateCartDiscountFn != nil {
		return m.calculateCartDiscountFn(ctx, promoID, items)
	}
	return nil, nil
}
func (m *mockPromoService) ValidatePromoLimits(ctx context.Context, promoID uuid.UUID, userID *uuid.UUID, email, phone *string) error {
	if m.validatePromoLimitsFn != nil {
		return m.validatePromoLimitsFn(ctx, promoID, userID, email, phone)
	}
	return nil
}

// --- Tests ---

func TestGetFullCart_WithFreeShipping(t *testing.T) {
	userID := uuid.New()
	variationID := uuid.New()
	cartID := uuid.New()

	cart := &domain.Cart{
		ID:     cartID,
		UserID: &userID,
		Items: []domain.CartItem{
			{CartID: cartID, VariationID: variationID, Quantity: 2},
		},
	}
	variations := []productDomain.ProductVariation{
		{ID: variationID, Price: 35000}, // 350 грн * 2 = 700 грн (> 600 threshold)
	}

	repo := &mockCartRepo{
		getByUserIDFn: func(ctx context.Context, id uuid.UUID, lang string) (*domain.Cart, []productDomain.ProductVariation, error) {
			return cart, variations, nil
		},
	}
	shipment := &mockShipmentService{threshold: 60000} // 600 грн

	svc := NewCartService(repo, shipment, &mockPromoService{}, &mockLogger{})
	_, _, shipping, _, _, err := svc.GetFullCart(context.Background(), &userID, nil, "uk")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !shipping.IsFreeShipping {
		t.Error("expected IsFreeShipping=true for 700 грн cart (threshold 600)")
	}
	if shipping.RemainingAmount != 0 {
		t.Errorf("expected RemainingAmount=0, got %d", shipping.RemainingAmount)
	}
	if shipping.Threshold != 60000 {
		t.Errorf("expected Threshold=60000, got %d", shipping.Threshold)
	}
}

func TestGetFullCart_WithoutFreeShipping(t *testing.T) {
	userID := uuid.New()
	variationID := uuid.New()
	cartID := uuid.New()

	cart := &domain.Cart{
		ID:     cartID,
		UserID: &userID,
		Items: []domain.CartItem{
			{CartID: cartID, VariationID: variationID, Quantity: 1},
		},
	}
	variations := []productDomain.ProductVariation{
		{ID: variationID, Price: 30000}, // 300 грн (< 600 threshold)
	}

	repo := &mockCartRepo{
		getByUserIDFn: func(ctx context.Context, id uuid.UUID, lang string) (*domain.Cart, []productDomain.ProductVariation, error) {
			return cart, variations, nil
		},
	}
	shipment := &mockShipmentService{threshold: 60000} // 600 грн

	svc := NewCartService(repo, shipment, &mockPromoService{}, &mockLogger{})
	_, _, shipping, _, _, err := svc.GetFullCart(context.Background(), &userID, nil, "uk")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if shipping.IsFreeShipping {
		t.Error("expected IsFreeShipping=false for 300 грн cart (threshold 600)")
	}
	if shipping.RemainingAmount != 30000 {
		t.Errorf("expected RemainingAmount=30000, got %d", shipping.RemainingAmount)
	}
}

func TestGetFullCart_ExactThreshold(t *testing.T) {
	userID := uuid.New()
	variationID := uuid.New()
	cartID := uuid.New()

	cart := &domain.Cart{
		ID:     cartID,
		UserID: &userID,
		Items: []domain.CartItem{
			{CartID: cartID, VariationID: variationID, Quantity: 2},
		},
	}
	variations := []productDomain.ProductVariation{
		{ID: variationID, Price: 30000}, // 300 * 2 = 600 грн = threshold
	}

	repo := &mockCartRepo{
		getByUserIDFn: func(ctx context.Context, id uuid.UUID, lang string) (*domain.Cart, []productDomain.ProductVariation, error) {
			return cart, variations, nil
		},
	}
	shipment := &mockShipmentService{threshold: 60000}

	svc := NewCartService(repo, shipment, &mockPromoService{}, &mockLogger{})
	_, _, shipping, _, _, err := svc.GetFullCart(context.Background(), &userID, nil, "uk")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !shipping.IsFreeShipping {
		t.Error("expected IsFreeShipping=true when total == threshold")
	}
}

func TestGetFullCart_EmptyCart_ShowsThreshold(t *testing.T) {
	shipment := &mockShipmentService{threshold: 60000}
	svc := NewCartService(&mockCartRepo{}, shipment, &mockPromoService{}, &mockLogger{})

	_, _, shipping, _, _, err := svc.GetFullCart(context.Background(), nil, nil, "uk")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if shipping.IsFreeShipping {
		t.Error("expected IsFreeShipping=false for empty cart")
	}
	if shipping.RemainingAmount != 60000 {
		t.Errorf("expected RemainingAmount=60000, got %d", shipping.RemainingAmount)
	}
}

func TestAddItem_Success(t *testing.T) {
	userID := uuid.New()
	variationID := uuid.New()

	repo := &mockCartRepo{
		variationExistsFn: func(ctx context.Context, vID uuid.UUID) (bool, error) {
			return true, nil
		},
		addItemFn: func(ctx context.Context, uid *uuid.UUID, sid *string, vID uuid.UUID, qty int) error {
			return nil
		},
	}

	svc := NewCartService(repo, &mockShipmentService{}, &mockPromoService{}, &mockLogger{})
	err := svc.AddItem(context.Background(), &userID, nil, variationID, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAddItem_VariationNotFound(t *testing.T) {
	userID := uuid.New()
	repo := &mockCartRepo{
		variationExistsFn: func(ctx context.Context, vID uuid.UUID) (bool, error) {
			return false, nil
		},
	}

	svc := NewCartService(repo, &mockShipmentService{}, &mockPromoService{}, &mockLogger{})
	err := svc.AddItem(context.Background(), &userID, nil, uuid.New(), 1)
	if err != domain.ErrVariationNotFound {
		t.Errorf("expected ErrVariationNotFound, got %v", err)
	}
}

func TestAddItem_InvalidQuantity(t *testing.T) {
	userID := uuid.New()
	svc := NewCartService(&mockCartRepo{}, &mockShipmentService{}, &mockPromoService{}, &mockLogger{})
	err := svc.AddItem(context.Background(), &userID, nil, uuid.New(), 0)
	if err != domain.ErrInvalidQuantity {
		t.Errorf("expected ErrInvalidQuantity, got %v", err)
	}
}

func TestAddItem_NoIdentifier(t *testing.T) {
	svc := NewCartService(&mockCartRepo{}, &mockShipmentService{}, &mockPromoService{}, &mockLogger{})
	err := svc.AddItem(context.Background(), nil, nil, uuid.New(), 1)
	if err != domain.ErrNoIdentifier {
		t.Errorf("expected ErrNoIdentifier, got %v", err)
	}
}

func TestSyncSession_Success(t *testing.T) {
	userID := uuid.New()
	sessionID := "session-to-sync"

	repo := &mockCartRepo{
		syncSessionToUserFn: func(ctx context.Context, sid string, uid uuid.UUID) error {
			return nil
		},
	}

	svc := NewCartService(repo, &mockShipmentService{}, &mockPromoService{}, &mockLogger{})
	err := svc.SyncSession(context.Background(), sessionID, userID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSyncSession_EmptySessionID(t *testing.T) {
	svc := NewCartService(&mockCartRepo{}, &mockShipmentService{}, &mockPromoService{}, &mockLogger{})
	err := svc.SyncSession(context.Background(), "", uuid.New())
	if err != domain.ErrNoIdentifier {
		t.Errorf("expected ErrNoIdentifier, got %v", err)
	}
}
