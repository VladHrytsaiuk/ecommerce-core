//go:build legacy && ignore
// +build legacy,ignore

package service

import (
	"context"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	productDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/product/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/wishlist/domain"
	"github.com/google/uuid"
)

type wishlistService struct {
	repo domain.WishlistRepository
	l    logger.Logger
}

// NewWishlistService створює новий інстанс сервісу
func NewWishlistService(repo domain.WishlistRepository, l logger.Logger) domain.WishlistService {
	return &wishlistService{repo: repo, l: l}
}

// GetItems повертає список варіацій з вішліста
func (s *wishlistService) GetItems(ctx context.Context, userID *uuid.UUID, sessionID *string, lang string) ([]productDomain.ProductVariation, error) {
	if userID != nil {
		return s.repo.GetByUserID(ctx, *userID, lang)
	}
	if sessionID != nil && *sessionID != "" {
		return s.repo.GetBySessionID(ctx, *sessionID, lang)
	}
	return nil, domain.ErrNoIdentifier
}

// AddItem додає варіацію до вішліста з валідацією
func (s *wishlistService) AddItem(ctx context.Context, userID *uuid.UUID, sessionID *string, variationID uuid.UUID) error {
	if userID == nil && (sessionID == nil || *sessionID == "") {
		return domain.ErrNoIdentifier
	}

	// Перевіряємо чи існує варіація
	exists, err := s.repo.VariationExists(ctx, variationID)
	if err != nil {
		return err
	}
	if !exists {
		return domain.ErrVariationNotFound
	}

	item := &domain.WishlistItem{
		UserID:      userID,
		SessionID:   sessionID,
		VariationID: variationID,
	}

	return s.repo.Add(ctx, item)
}

// RemoveItem видаляє варіацію з вішліста
func (s *wishlistService) RemoveItem(ctx context.Context, userID *uuid.UUID, sessionID *string, variationID uuid.UUID) error {
	return s.repo.Remove(ctx, userID, sessionID, variationID)
}

// SyncSession переносить анонімний вішліст на авторизованого юзера
func (s *wishlistService) SyncSession(ctx context.Context, sessionID string, userID uuid.UUID) error {
	if sessionID == "" {
		return domain.ErrNoIdentifier
	}
	s.l.Infow("Starting wishlist sync", "session_id", sessionID, "user_id", userID)
	return s.repo.SyncSessionToUser(ctx, sessionID, userID)
}
