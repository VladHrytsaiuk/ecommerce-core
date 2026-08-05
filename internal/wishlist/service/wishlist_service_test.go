//go:build legacy && ignore
// +build legacy,ignore

package service

import (
	"context"
	"testing"
	"time"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	productDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/product/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/wishlist/domain"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// --- Mock Repository ---

type mockWishlistRepo struct {
	getByUserIDFn            func(ctx context.Context, userID uuid.UUID, lang string) ([]productDomain.ProductVariation, error)
	getBySessionIDFn         func(ctx context.Context, sessionID string, lang string) ([]productDomain.ProductVariation, error)
	addFn                    func(ctx context.Context, item *domain.WishlistItem) error
	removeFn                 func(ctx context.Context, userID *uuid.UUID, sessionID *string, variationID uuid.UUID) error
	syncSessionToUserFn      func(ctx context.Context, sessionID string, userID uuid.UUID) error
	deleteExpiredAnonymousFn func(ctx context.Context, olderThan time.Time) error
	variationExistsFn        func(ctx context.Context, variationID uuid.UUID) (bool, error)
}

func (m *mockWishlistRepo) GetByUserID(ctx context.Context, userID uuid.UUID, lang string) ([]productDomain.ProductVariation, error) {
	return m.getByUserIDFn(ctx, userID, lang)
}
func (m *mockWishlistRepo) GetBySessionID(ctx context.Context, sessionID string, lang string) ([]productDomain.ProductVariation, error) {
	return m.getBySessionIDFn(ctx, sessionID, lang)
}
func (m *mockWishlistRepo) Add(ctx context.Context, item *domain.WishlistItem) error {
	return m.addFn(ctx, item)
}
func (m *mockWishlistRepo) Remove(ctx context.Context, userID *uuid.UUID, sessionID *string, variationID uuid.UUID) error {
	return m.removeFn(ctx, userID, sessionID, variationID)
}
func (m *mockWishlistRepo) SyncSessionToUser(ctx context.Context, sessionID string, userID uuid.UUID) error {
	return m.syncSessionToUserFn(ctx, sessionID, userID)
}
func (m *mockWishlistRepo) DeleteExpiredAnonymous(ctx context.Context, olderThan time.Time) error {
	return m.deleteExpiredAnonymousFn(ctx, olderThan)
}
func (m *mockWishlistRepo) VariationExists(ctx context.Context, variationID uuid.UUID) (bool, error) {
	return m.variationExistsFn(ctx, variationID)
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

// --- Tests ---

func TestGetItems_AuthenticatedUser(t *testing.T) {
	userID := uuid.New()
	expectedVariations := []productDomain.ProductVariation{
		{ID: uuid.New(), ProductID: uuid.New(), Price: 100},
	}

	repo := &mockWishlistRepo{
		getByUserIDFn: func(ctx context.Context, id uuid.UUID, lang string) ([]productDomain.ProductVariation, error) {
			if id != userID {
				t.Errorf("expected user_id %s, got %s", userID, id)
			}
			if lang != "uk" {
				t.Errorf("expected lang 'uk', got '%s'", lang)
			}
			return expectedVariations, nil
		},
	}

	svc := NewWishlistService(repo, &mockLogger{})
	variations, err := svc.GetItems(context.Background(), &userID, nil, "uk")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(variations) != 1 {
		t.Fatalf("expected 1 variation, got %d", len(variations))
	}
	if variations[0].Price != 100 {
		t.Errorf("expected price 100, got %d", variations[0].Price)
	}
}

func TestGetItems_AnonymousSession(t *testing.T) {
	sessionID := "test-session-123"
	expectedVariations := []productDomain.ProductVariation{
		{ID: uuid.New(), ProductID: uuid.New(), Price: 200},
	}

	repo := &mockWishlistRepo{
		getBySessionIDFn: func(ctx context.Context, sid string, lang string) ([]productDomain.ProductVariation, error) {
			if sid != sessionID {
				t.Errorf("expected session_id %s, got %s", sessionID, sid)
			}
			return expectedVariations, nil
		},
	}

	svc := NewWishlistService(repo, &mockLogger{})
	variations, err := svc.GetItems(context.Background(), nil, &sessionID, "uk")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(variations) != 1 {
		t.Fatalf("expected 1 variation, got %d", len(variations))
	}
}

func TestGetItems_NoIdentifier(t *testing.T) {
	svc := NewWishlistService(&mockWishlistRepo{}, &mockLogger{})
	_, err := svc.GetItems(context.Background(), nil, nil, "uk")
	if err != domain.ErrNoIdentifier {
		t.Errorf("expected ErrNoIdentifier, got %v", err)
	}
}

func TestAddItem_Success(t *testing.T) {
	userID := uuid.New()
	variationID := uuid.New()

	repo := &mockWishlistRepo{
		variationExistsFn: func(ctx context.Context, vID uuid.UUID) (bool, error) {
			return true, nil
		},
		addFn: func(ctx context.Context, item *domain.WishlistItem) error {
			if *item.UserID != userID {
				t.Errorf("expected user_id %s, got %s", userID, *item.UserID)
			}
			if item.VariationID != variationID {
				t.Errorf("expected variation_id %s, got %s", variationID, item.VariationID)
			}
			return nil
		},
	}

	svc := NewWishlistService(repo, &mockLogger{})
	err := svc.AddItem(context.Background(), &userID, nil, variationID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAddItem_VariationNotFound(t *testing.T) {
	userID := uuid.New()

	repo := &mockWishlistRepo{
		variationExistsFn: func(ctx context.Context, vID uuid.UUID) (bool, error) {
			return false, nil
		},
	}

	svc := NewWishlistService(repo, &mockLogger{})
	err := svc.AddItem(context.Background(), &userID, nil, uuid.New())
	if err != domain.ErrVariationNotFound {
		t.Errorf("expected ErrVariationNotFound, got %v", err)
	}
}

func TestAddItem_NoIdentifier(t *testing.T) {
	svc := NewWishlistService(&mockWishlistRepo{}, &mockLogger{})
	err := svc.AddItem(context.Background(), nil, nil, uuid.New())
	if err != domain.ErrNoIdentifier {
		t.Errorf("expected ErrNoIdentifier, got %v", err)
	}
}

func TestRemoveItem_Success(t *testing.T) {
	userID := uuid.New()
	variationID := uuid.New()

	repo := &mockWishlistRepo{
		removeFn: func(ctx context.Context, uid *uuid.UUID, sid *string, vID uuid.UUID) error {
			if *uid != userID {
				t.Errorf("expected user_id %s, got %s", userID, *uid)
			}
			if vID != variationID {
				t.Errorf("expected variation_id %s, got %s", variationID, vID)
			}
			return nil
		},
	}

	svc := NewWishlistService(repo, &mockLogger{})
	err := svc.RemoveItem(context.Background(), &userID, nil, variationID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSyncSession_Success(t *testing.T) {
	userID := uuid.New()
	sessionID := "session-to-sync"

	repo := &mockWishlistRepo{
		syncSessionToUserFn: func(ctx context.Context, sid string, uid uuid.UUID) error {
			if sid != sessionID {
				t.Errorf("expected session_id %s, got %s", sessionID, sid)
			}
			if uid != userID {
				t.Errorf("expected user_id %s, got %s", userID, uid)
			}
			return nil
		},
	}

	svc := NewWishlistService(repo, &mockLogger{})
	err := svc.SyncSession(context.Background(), sessionID, userID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSyncSession_EmptySessionID(t *testing.T) {
	svc := NewWishlistService(&mockWishlistRepo{}, &mockLogger{})
	err := svc.SyncSession(context.Background(), "", uuid.New())
	if err != domain.ErrNoIdentifier {
		t.Errorf("expected ErrNoIdentifier, got %v", err)
	}
}
