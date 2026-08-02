package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Product is the clean catalog aggregate. Localized business content belongs
// only to ProductTranslation records, never to JSONB fields on the product.
type Product struct {
	ID           uuid.UUID            `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	CategoryID   *uuid.UUID           `gorm:"type:uuid" json:"category_id,omitempty"`
	Status       string               `gorm:"type:varchar(32);not null" json:"status"`
	CreatedAt    time.Time            `json:"created_at"`
	UpdatedAt    time.Time            `json:"updated_at"`
	Translations []ProductTranslation `gorm:"foreignKey:ProductID;constraint:OnDelete:CASCADE" json:"translations"`
}

func (Product) TableName() string { return "products" }

type ProductTranslation struct {
	ID          uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	ProductID   uuid.UUID `gorm:"type:uuid;not null;index" json:"product_id"`
	Locale      string    `gorm:"type:varchar(10);not null" json:"locale"`
	Name        string    `gorm:"type:text;not null" json:"name"`
	Description string    `gorm:"type:text" json:"description,omitempty"`
	Slug        string    `gorm:"type:varchar(255);not null" json:"slug"`
}

func (ProductTranslation) TableName() string { return "product_translations" }

type ProductRepository interface {
	FindBySlug(context.Context, string, string) (*Product, error)
	Create(context.Context, *Product) error
}
