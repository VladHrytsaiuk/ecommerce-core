//go:build legacy && ignore
// +build legacy,ignore

package domain

import (
	"context"
	"errors"
	"time"

	productDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/product/domain"
	"github.com/google/uuid"
)

var (
	ErrItemAlreadyInWishlist = errors.New("item already in wishlist")
	ErrItemNotInWishlist     = errors.New("item not found in wishlist")
	ErrVariationNotFound     = errors.New("product variation not found")
	ErrNoIdentifier          = errors.New("either user_id or session_id must be provided")
)

// WishlistItem відповідає таблиці wishlist у БД
type WishlistItem struct {
	UserID      *uuid.UUID `gorm:"type:uuid" json:"user_id"`
	SessionID   *string    `gorm:"type:varchar(255)" json:"session_id"`
	VariationID uuid.UUID  `gorm:"type:uuid;not null" json:"variation_id"`
	CreatedAt   time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`

	// Relationships
	Variation productDomain.ProductVariation `gorm:"foreignKey:VariationID" json:"variation,omitempty"`
}

func (WishlistItem) TableName() string {
	return "wishlist"
}

// WishlistRepository контракт для роботи з БД
type WishlistRepository interface {
	// GetByUserID повертає список варіацій з вішліста для авторизованого юзера
	GetByUserID(ctx context.Context, userID uuid.UUID, lang string) ([]productDomain.ProductVariation, error)
	// GetBySessionID повертає список варіацій з вішліста для анонімної сесії
	GetBySessionID(ctx context.Context, sessionID string, lang string) ([]productDomain.ProductVariation, error)

	// Add додає variation до вішліста (upsert — ігнорує конфлікт якщо вже є)
	Add(ctx context.Context, item *WishlistItem) error
	// Remove видаляє variation з вішліста
	Remove(ctx context.Context, userID *uuid.UUID, sessionID *string, variationID uuid.UUID) error

	// SyncSessionToUser переносить усі items з session на user (з дедуплікацією)
	SyncSessionToUser(ctx context.Context, sessionID string, userID uuid.UUID) error

	// DeleteExpiredAnonymous видаляє анонімні записи старші за вказаний час
	DeleteExpiredAnonymous(ctx context.Context, olderThan time.Time) error

	// VariationExists перевіряє чи існує варіація
	VariationExists(ctx context.Context, variationID uuid.UUID) (bool, error)
}

// WishlistService контракт для бізнес-логіки
type WishlistService interface {
	// GetItems повертає список варіацій з вішліста
	GetItems(ctx context.Context, userID *uuid.UUID, sessionID *string, lang string) ([]productDomain.ProductVariation, error)
	// AddItem додає товар до вішліста
	AddItem(ctx context.Context, userID *uuid.UUID, sessionID *string, variationID uuid.UUID) error
	// RemoveItem видаляє товар з вішліста
	RemoveItem(ctx context.Context, userID *uuid.UUID, sessionID *string, variationID uuid.UUID) error
	// SyncSession переносить анонімний вішліст на авторизованого юзера
	SyncSession(ctx context.Context, sessionID string, userID uuid.UUID) error
}
