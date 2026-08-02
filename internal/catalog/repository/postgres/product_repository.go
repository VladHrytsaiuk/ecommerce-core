package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/domain"
)

type ProductRepository struct{ db *gorm.DB }

func NewProductRepository(db *gorm.DB) *ProductRepository { return &ProductRepository{db: db} }

func (r *ProductRepository) FindBySlug(ctx context.Context, locale, slug string) (*domain.Product, error) {
	var product domain.Product
	err := r.db.WithContext(ctx).
		Joins("JOIN product_translations pt ON pt.product_id = products.id").
		Preload("Translations").
		Where("pt.locale = ? AND pt.slug = ?", locale, slug).
		First(&product).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrProductNotFound
		}
		return nil, err
	}
	return &product, nil
}

func (r *ProductRepository) Create(ctx context.Context, product *domain.Product) error {
	if product.ID == uuid.Nil {
		product.ID = uuid.New()
	}
	for i := range product.Translations {
		product.Translations[i].ProductID = product.ID
	}
	return r.db.WithContext(ctx).Create(product).Error
}
