package postgres

import (
	"context"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/sitemap/domain"
	"gorm.io/gorm"
)

type sitemapRepository struct {
	db *gorm.DB
}

func NewSitemapRepository(db *gorm.DB) domain.SitemapRepository {
	return &sitemapRepository{db: db}
}

func (r *sitemapRepository) GetActiveProductSlugs(ctx context.Context) ([]domain.EntityInfo, error) {
	var results []domain.EntityInfo
	err := r.db.WithContext(ctx).
		Table("product").
		Joins("JOIN product_translation pt ON pt.product_id = product.id").
		Select("pt.slug, pt.language_code, product.updated_at").
		Where("product.is_active = ? AND product.deleted_at IS NULL", true).
		Find(&results).Error
	return results, err
}

func (r *sitemapRepository) GetCategorySlugs(ctx context.Context) ([]domain.EntityInfo, error) {
	var results []domain.EntityInfo
	err := r.db.WithContext(ctx).
		Table("category").
		Joins("JOIN category_translation ct ON ct.category_id = category.id").
		Select("ct.slug, ct.language_code, category.updated_at").
		Where("category.deleted_at IS NULL").
		Find(&results).Error
	return results, err
}

func (r *sitemapRepository) GetBrandSlugs(ctx context.Context) ([]domain.EntityInfo, error) {
	var results []domain.EntityInfo
	err := r.db.WithContext(ctx).
		Table("brand").
		Select("slug, updated_at").
		Where("deleted_at IS NULL").
		Find(&results).Error
	return results, err
}

func (r *sitemapRepository) GetDocumentSlugs(ctx context.Context) ([]domain.EntityInfo, error) {
	var results []domain.EntityInfo
	err := r.db.WithContext(ctx).
		Table("documents").
		Select("slug, updated_at").
		Where("deleted_at IS NULL").
		Find(&results).Error
	return results, err
}
