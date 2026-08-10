package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/postgres/transaction"
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

func (r *ProductRepository) List(ctx context.Context) ([]domain.Product, error) {
	var products []domain.Product
	if err := r.db.WithContext(ctx).Preload("Translations").Order("created_at DESC").Find(&products).Error; err != nil {
		return nil, err
	}
	return products, nil
}

func (r *ProductRepository) Create(ctx context.Context, product *domain.Product) error {
	if product.ID == uuid.Nil {
		product.ID = uuid.New()
	}
	for i := range product.Translations {
		product.Translations[i].ProductID = product.ID
	}
	return r.database(ctx).Create(product).Error
}

func (r *ProductRepository) Update(ctx context.Context, product *domain.Product) error {
	for i := range product.Translations {
		product.Translations[i].ProductID = product.ID
	}
	db := r.database(ctx)
	if err := db.Model(&domain.Product{}).Where("id = ?", product.ID).Updates(map[string]any{"category_id": product.CategoryID, "status": product.Status, "updated_at": gorm.Expr("CURRENT_TIMESTAMP")}).Error; err != nil {
		return err
	}
	if err := db.Where("product_id = ?", product.ID).Delete(&domain.ProductTranslation{}).Error; err != nil {
		return err
	}
	return db.Create(&product.Translations).Error
}

func (r *ProductRepository) FindByIDForUpdate(ctx context.Context, id uuid.UUID) (*domain.Product, error) {
	var product domain.Product
	if err := r.database(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Preload("Translations").First(&product, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &product, nil
}

func (r *ProductRepository) database(ctx context.Context) *gorm.DB {
	if tx, err := transaction.FromContext(ctx); err == nil {
		return tx.WithContext(ctx)
	}
	return r.db.WithContext(ctx)
}

var _ domain.AdminProductRepository = (*ProductRepository)(nil)
