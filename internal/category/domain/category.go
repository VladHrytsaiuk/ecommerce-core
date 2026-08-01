package domain

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	ErrCategoryNotFound = errors.New("category not found")
	ErrCategoryInUse    = errors.New("cannot delete category: it is linked to existing products")
)

// ProductChecker контракт для перевірки прив'язки продуктів
type ProductChecker interface {
	CountByCategory(ctx context.Context, categoryID uuid.UUID) (int64, error)
	GetActiveCategoryIDs(ctx context.Context) (map[uuid.UUID]bool, error)
}

// Category представляє категорію товарів
type Category struct {
	ID        uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	ParentID  *uuid.UUID     `gorm:"type:uuid;index" json:"parent_id"`
	CreatedAt time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
	IconURL   *string        `gorm:"type:varchar(255)" json:"icon_url"`
	SortOrder int            `gorm:"not null;default:0" json:"sort_order"`

	// Relationships
	Translations []CategoryTranslation `gorm:"foreignKey:CategoryID" json:"translations,omitempty"`
}

func (Category) TableName() string {
	return "category"
}

// CategoryTranslation містить локалізовані дані для категорій
type CategoryTranslation struct {
	CategoryID   uuid.UUID `gorm:"type:uuid;primaryKey" json:"category_id"`
	LanguageCode string    `gorm:"type:varchar(2);primaryKey" json:"language_code"`
	Name            string    `gorm:"type:varchar(100);not null" json:"name"`
	Slug            string    `gorm:"type:varchar(255);not null;unique" json:"slug"`
	MetaTitle       string    `gorm:"type:varchar(255)" json:"meta_title"`
	MetaDescription string    `gorm:"type:text" json:"meta_description"`
	MetaKeywords    string    `gorm:"type:varchar(255)" json:"meta_keywords"`
}

func (CategoryTranslation) TableName() string {
	return "category_translation"
}

// CategoryRepository контракт для роботи з БД
type CategoryRepository interface {
	FindAll(ctx context.Context, lang string) ([]Category, error)
	FindByID(ctx context.Context, id uuid.UUID, lang string) (*Category, error)
	FindBySlug(ctx context.Context, slug string, lang string) (*Category, error)
	Create(ctx context.Context, category *Category) error
	Update(ctx context.Context, category *Category) error
	Delete(ctx context.Context, id uuid.UUID) error
	UpdateOrder(ctx context.Context, ids []uuid.UUID) error
	SlugExists(ctx context.Context, slug string, excludeID uuid.UUID) (bool, error)
}

// CategoryService контракт для бізнес-логіки
type CategoryService interface {
	GetList(ctx context.Context, lang string, showAll bool) ([]Category, error)
	GenerateSlug(ctx context.Context, name string, excludeID uuid.UUID) (string, error)
	GetByID(ctx context.Context, id uuid.UUID, lang string) (*Category, error)
	GetBySlug(ctx context.Context, slug string, lang string) (*Category, error)
	Create(ctx context.Context, category *Category) error
	Update(ctx context.Context, category *Category) error
	Delete(ctx context.Context, id uuid.UUID) error
	UpdateOrder(ctx context.Context, ids []uuid.UUID) error
}
