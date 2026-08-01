package domain

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	discountDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/discount/domain"
	productDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/product/domain"
)

var (
	ErrCartNotFound      = errors.New("cart not found")
	ErrCartItemNotFound  = errors.New("cart item not found")
	ErrVariationNotFound = errors.New("product variation not found")
	ErrNoIdentifier      = errors.New("either user_id or session_id must be provided")
	ErrInvalidQuantity   = errors.New("quantity must be greater than 0")
)

// Cart заголовок кошика
type Cart struct {
	ID        uuid.UUID  `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	UserID    *uuid.UUID `gorm:"type:uuid" json:"user_id"`
	SessionID *string    `gorm:"type:varchar(255)" json:"session_id"`
	PromoCodeID *uuid.UUID `gorm:"type:uuid" json:"promo_code_id"`
	CreatedAt time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`

	// Relationships
	Items []CartItem `gorm:"foreignKey:CartID" json:"items,omitempty"`
}

func (Cart) TableName() string {
	return "cart"
}

// CartItem елемент кошика
type CartItem struct {
	ID          uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	CartID      uuid.UUID `gorm:"type:uuid;not null" json:"cart_id"`
	VariationID uuid.UUID `gorm:"type:uuid;not null" json:"variation_id"`
	Quantity    int       `gorm:"not null;default:1" json:"quantity"`
	CreatedAt   time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt   time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`

	// Relationships
	Variation productDomain.ProductVariation `gorm:"foreignKey:VariationID" json:"variation,omitempty"`
}

func (CartItem) TableName() string {
	return "cart_item"
}

// CartRepository контракт для роботи з БД
type CartRepository interface {
	// GetByUserID повертає кошик з усіма items для авторизованого юзера
	GetByUserID(ctx context.Context, userID uuid.UUID, lang string) (*Cart, []productDomain.ProductVariation, error)
	// GetBySessionID повертає кошик з усіма items для анонімної сесії
	GetBySessionID(ctx context.Context, sessionID string, lang string) (*Cart, []productDomain.ProductVariation, error)

	// AddItem додає variation до кошика (upsert — якщо вже є, збільшує quantity)
	AddItem(ctx context.Context, userID *uuid.UUID, sessionID *string, variationID uuid.UUID, quantity int) error
	// UpdateQuantity встановлює конкретну кількість для item
	UpdateQuantity(ctx context.Context, userID *uuid.UUID, sessionID *string, variationID uuid.UUID, quantity int) error
	// RemoveItem видаляє variation з кошика
	RemoveItem(ctx context.Context, userID *uuid.UUID, sessionID *string, variationID uuid.UUID) error

	// SyncSessionToUser переносить усі items з session-кошика на user-кошик (з додаванням кількостей)
	SyncSessionToUser(ctx context.Context, sessionID string, userID uuid.UUID) error

	// UpdatePromoCode оновлює промокод для кошика
	UpdatePromoCode(ctx context.Context, userID *uuid.UUID, sessionID *string, promoCodeID *uuid.UUID) error

	// DeleteExpiredAnonymous видаляє анонімні кошики старші за вказаний час
	DeleteExpiredAnonymous(ctx context.Context, olderThan time.Time) error

	// VariationExists перевіряє чи існує варіація
	VariationExists(ctx context.Context, variationID uuid.UUID) (bool, error)
}

// CartService контракт для бізнес-логіки
type CartService interface {
	// GetFullCart повертає кошик з усіма даними варіацій та інформацією про доставку
	GetFullCart(ctx context.Context, userID *uuid.UUID, sessionID *string, lang string) (*Cart, []productDomain.ProductVariation, *ShippingSummary, *discountDomain.PromoCalculationResult, *string, error)
	// AddItem додає товар до кошика
	AddItem(ctx context.Context, userID *uuid.UUID, sessionID *string, variationID uuid.UUID, quantity int) error
	// UpdateQuantity змінює кількість товару в кошику
	UpdateQuantity(ctx context.Context, userID *uuid.UUID, sessionID *string, variationID uuid.UUID, quantity int) error
	// RemoveItem видаляє товар з кошика
	RemoveItem(ctx context.Context, userID *uuid.UUID, sessionID *string, variationID uuid.UUID) error
	// SyncSession переносить анонімний кошик на авторизованого юзера
	SyncSession(ctx context.Context, sessionID string, userID uuid.UUID) error

	// ApplyPromoCode застосовує промокод до кошика
	ApplyPromoCode(ctx context.Context, userID *uuid.UUID, sessionID *string, code string, lang string) error
	// RemovePromoCode видаляє промокод з кошика
	RemovePromoCode(ctx context.Context, userID *uuid.UUID, sessionID *string) error
}

// CartSummary підсумки кошика (для внутрішніх розрахунків)
type CartSummary struct {
	TotalCount int
	TotalPrice int
}

// ShippingSummary інформація про безкоштовну доставку для фронтенду
type ShippingSummary struct {
	IsFreeShipping  bool `json:"is_free_shipping"`
	Threshold       int  `json:"threshold"`        // мінімальна сума для безкоштовної доставки (копійки)
	RemainingAmount int  `json:"remaining_amount"` // скільки ще потрібно додати (копійки)
	MinOrderAmount  int  `json:"min_order_amount"` // мінімальна сума замовлення (копійки)
}

// CartItemDetail деталі одного елемента кошика з інформацією про варіацію
type CartItemDetail struct {
	VariationID uuid.UUID `json:"variation_id"`
	Quantity    int       `json:"quantity"`
	ItemPrice   int       `json:"item_price"`
	TotalPrice  int       `json:"total_price"`
}
