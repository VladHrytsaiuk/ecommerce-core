package domain

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrCatalogCategoryNotFound = errors.New("catalog category not found")
	ErrInvalidCatalogCategory  = errors.New("invalid catalog category")
)

// Category is the clean Catalog aggregate. It deliberately has no language
// columns; all storefront content belongs to CategoryTranslation.
type Category struct {
	ID           uuid.UUID             `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	ParentID     *uuid.UUID            `gorm:"type:uuid" json:"parent_id,omitempty"`
	SortOrder    int                   `gorm:"not null;default:0" json:"sort_order"`
	IsActive     bool                  `gorm:"not null;default:true" json:"is_active"`
	CreatedAt    time.Time             `json:"created_at"`
	UpdatedAt    time.Time             `json:"updated_at"`
	Translations []CategoryTranslation `gorm:"foreignKey:CategoryID;constraint:OnDelete:CASCADE" json:"translations"`
}

func (Category) TableName() string { return "categories" }

type CategoryTranslation struct {
	ID          uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	CategoryID  uuid.UUID `gorm:"type:uuid;not null;index" json:"category_id"`
	Locale      string    `gorm:"type:varchar(10);not null" json:"locale"`
	Name        string    `gorm:"type:text;not null" json:"name"`
	Description string    `gorm:"type:text" json:"description,omitempty"`
	Slug        string    `gorm:"type:varchar(255);not null" json:"slug"`
}

func (CategoryTranslation) TableName() string { return "category_translations" }

type CategoryRepository interface {
	FindBySlug(context.Context, string, string) (*Category, error)
	Create(context.Context, *Category) error
}

type CategoryService interface {
	FindBySlug(context.Context, string, string) (*Category, error)
	Create(context.Context, *Category) error
}

type AdminCategoryRepository interface {
	Update(context.Context, *Category) error
}
