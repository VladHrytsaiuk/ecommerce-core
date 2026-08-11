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

func (r *ProductRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.Product, error) {
	var product domain.Product
	if err := r.database(ctx).Preload("Translations").Preload("Media").First(&product, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrProductNotFound
		}
		return nil, err
	}
	return &product, nil
}

// ListActiveAfter uses keyset pagination so maintenance reindexing remains
// O(number of products), rather than making PostgreSQL repeatedly discard a
// growing OFFSET prefix.
func (r *ProductRepository) ListActiveAfter(ctx context.Context, after *uuid.UUID, limit int) ([]domain.Product, error) {
	var products []domain.Product
	query := r.database(ctx).Preload("Translations").Where("status = ?", "active")
	if after != nil && *after != uuid.Nil {
		query = query.Where("id > ?", *after)
	}
	if err := query.Order("id ASC").Limit(limit).Find(&products).Error; err != nil {
		return nil, err
	}
	return products, nil
}

func (r *ProductRepository) FindBySlug(ctx context.Context, locale, slug string) (*domain.Product, error) {
	var product domain.Product
	err := r.database(ctx).
		Joins("JOIN product_translations pt ON pt.product_id = products.id").
		Preload("Translations").Preload("Media").
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
	if err := r.database(ctx).Preload("Translations").Preload("Media").Order("created_at DESC").Find(&products).Error; err != nil {
		return nil, err
	}
	return products, nil
}

// ListProducts applies LIMIT/OFFSET in PostgreSQL and counts the same
// locale-visible product set. A product without a translation for the
// requested locale is deliberately not exposed through that localized API.
func (r *ProductRepository) ListProducts(ctx context.Context, locale string, page, limit int) ([]domain.Product, int64, error) {
	db := r.database(ctx)
	visible := db.Model(&domain.Product{}).
		Joins("JOIN product_translations pt ON pt.product_id = products.id").
		Where("pt.locale = ?", locale)

	var total int64
	if err := visible.Distinct("products.id").Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var products []domain.Product
	offset := (page - 1) * limit
	if err := visible.
		Distinct("products.*").
		Preload("Translations").Preload("Media").
		Order("products.created_at DESC, products.id DESC").
		Limit(limit).
		Offset(offset).
		Find(&products).Error; err != nil {
		return nil, 0, err
	}
	return products, total, nil
}

func (r *ProductRepository) Create(ctx context.Context, product *domain.Product) error {
	if product.ID == uuid.Nil {
		product.ID = uuid.New()
	}
	for i := range product.Translations {
		product.Translations[i].ProductID = product.ID
	}
	db := r.database(ctx)
	if err := db.Create(product).Error; err != nil {
		return err
	}
	return r.replaceMedia(db, product)
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
	if err := db.Create(&product.Translations).Error; err != nil {
		return err
	}
	return r.replaceMedia(db, product)
}

func (r *ProductRepository) Delete(ctx context.Context, id uuid.UUID) error {
	result := r.database(ctx).Delete(&domain.Product{}, "id = ?", id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrProductNotFound
	}
	return nil
}

func (r *ProductRepository) FindByIDForUpdate(ctx context.Context, id uuid.UUID) (*domain.Product, error) {
	var product domain.Product
	if err := r.database(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Preload("Translations").Preload("Media").First(&product, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &product, nil
}
func (r *ProductRepository) replaceMedia(db *gorm.DB, p *domain.Product) error {
	if p.Media == nil {
		return nil
	}
	if err := db.Where("product_id=?", p.ID).Delete(&domain.ProductMedia{}).Error; err != nil {
		return err
	}
	for i := range p.Media {
		p.Media[i].ProductID = p.ID
	}
	if len(p.Media) == 0 {
		return nil
	}
	return db.Create(&p.Media).Error
}

func (r *ProductRepository) database(ctx context.Context) *gorm.DB {
	if tx, err := transaction.FromContext(ctx); err == nil {
		return tx.WithContext(ctx)
	}
	return r.db.WithContext(ctx)
}

var _ domain.AdminProductRepository = (*ProductRepository)(nil)
