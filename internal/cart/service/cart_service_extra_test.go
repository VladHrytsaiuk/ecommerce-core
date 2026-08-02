//go:build legacy
// +build legacy

package service

import (
	"context"
	"testing"
	"time"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/cart/domain"
	discountDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/discount/domain"
	productDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/product/domain"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCartService_UpdateQuantity(t *testing.T) {
	repo := &mockCartRepo{
		updateQuantityFn: func(ctx context.Context, userID *uuid.UUID, sessionID *string, variationID uuid.UUID, quantity int) error {
			return nil
		},
	}
	svc := NewCartService(repo, nil, nil, &mockLogger{})

	userID := uuid.New()
	err := svc.UpdateQuantity(context.Background(), &userID, nil, uuid.New(), 5)
	assert.NoError(t, err)
}

func TestCartService_RemoveItem(t *testing.T) {
	repo := &mockCartRepo{
		removeItemFn: func(ctx context.Context, userID *uuid.UUID, sessionID *string, variationID uuid.UUID) error {
			return nil
		},
	}
	svc := NewCartService(repo, nil, nil, &mockLogger{})

	userID := uuid.New()
	err := svc.RemoveItem(context.Background(), &userID, nil, uuid.New())
	assert.NoError(t, err)
}

func TestCartService_ApplyPromoCode(t *testing.T) {
	promoID := uuid.New()
	promo := &discountDomain.PromoCode{ID: promoID, Code: "TEST10"}

	repo := &mockCartRepo{
		getByUserIDFn: func(ctx context.Context, id uuid.UUID, lang string) (*domain.Cart, []productDomain.ProductVariation, error) {
			return &domain.Cart{Items: []domain.CartItem{{VariationID: uuid.New(), Quantity: 1}}}, []productDomain.ProductVariation{{ID: uuid.New(), Price: 1000}}, nil
		},
		updatePromoCodeFn: func(ctx context.Context, userID *uuid.UUID, sessionID *string, promoCodeID *uuid.UUID) error {
			return nil
		},
	}
	promoService := &mockPromoService{
		getPromoByCodeFn: func(ctx context.Context, code string) (*discountDomain.PromoCode, error) {
			if code == "TEST10" {
				return promo, nil
			}
			return nil, discountDomain.ErrPromoCodeNotFound
		},
		validatePromoLimitsFn: func(ctx context.Context, promoID uuid.UUID, userID *uuid.UUID, email, phone *string) error {
			return nil
		},
	}
	svc := NewCartService(repo, nil, promoService, &mockLogger{})

	userID := uuid.New()
	err := svc.ApplyPromoCode(context.Background(), &userID, nil, "TEST10", "uk")
	assert.NoError(t, err)

	err = svc.ApplyPromoCode(context.Background(), &userID, nil, "INVALID", "uk")
	assert.ErrorIs(t, err, discountDomain.ErrPromoCodeNotFound)
}

func TestCartService_RemovePromoCode(t *testing.T) {
	repo := &mockCartRepo{
		getByUserIDFn: func(ctx context.Context, id uuid.UUID, lang string) (*domain.Cart, []productDomain.ProductVariation, error) {
			return &domain.Cart{}, nil, nil
		},
		updatePromoCodeFn: func(ctx context.Context, userID *uuid.UUID, sessionID *string, promoCodeID *uuid.UUID) error {
			return nil
		},
	}
	svc := NewCartService(repo, nil, nil, &mockLogger{})

	userID := uuid.New()
	err := svc.RemovePromoCode(context.Background(), &userID, nil)
	assert.NoError(t, err)
}

func TestGetFullCart_WithSessionID(t *testing.T) {
	sessionID := "sess-123"
	variationID := uuid.New()
	cartID := uuid.New()
	promoID := uuid.New()

	cart := &domain.Cart{
		ID:          cartID,
		SessionID:   &sessionID,
		PromoCodeID: &promoID,
		Items: []domain.CartItem{
			{CartID: cartID, VariationID: variationID, Quantity: 1},
		},
	}
	variations := []productDomain.ProductVariation{
		{ID: variationID, Price: 10000}, // 100 грн
	}

	repo := &mockCartRepo{
		getBySessionIDFn: func(ctx context.Context, sid string, lang string) (*domain.Cart, []productDomain.ProductVariation, error) {
			return cart, variations, nil
		},
	}
	shipment := &mockShipmentService{threshold: 60000} // 600 грн (нема безкоштовної доставки)
	promoService := &mockPromoService{
		getPromoByIDFn: func(ctx context.Context, id uuid.UUID) (*discountDomain.PromoCode, error) {
			return &discountDomain.PromoCode{ID: id, DiscountType: "PERCENT", DiscountValue: 10}, nil
		},
		calculateCartDiscountFn: func(ctx context.Context, id uuid.UUID, items []discountDomain.PromoItemInfo) (*discountDomain.PromoCalculationResult, error) {
			return &discountDomain.PromoCalculationResult{
				TotalDiscountAmount: 1000,
				ItemDiscounts:       map[uuid.UUID]int{variationID: 1000},
			}, nil
		},
	}

	svc := NewCartService(repo, shipment, promoService, &mockLogger{})
	resCart, resVars, shipping, promoCalc, promoCodeStr, err := svc.GetFullCart(context.Background(), nil, &sessionID, "uk")
	require.NoError(t, err)
	assert.NotNil(t, resCart)
	assert.NotNil(t, resVars)
	assert.Equal(t, 51000, shipping.RemainingAmount)
	assert.Equal(t, 1000, promoCalc.TotalDiscountAmount)
	require.NotNil(t, promoCodeStr)
	assert.Equal(t, "", *promoCodeStr)
}

func TestCartCleanupWorker(t *testing.T) {
	repo := &mockCartRepo{
		deleteExpiredAnonymousFn: func(ctx context.Context, olderThan time.Time) error {
			return nil
		},
	}
	worker := NewCartCleanupWorker(repo, &mockLogger{})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		worker.Run(ctx, time.Millisecond)
		close(done)
	}()
	time.Sleep(5 * time.Millisecond) // Let it run
	cancel()
	<-done
}
