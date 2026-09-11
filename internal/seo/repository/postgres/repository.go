// Package postgres implements the optional SEO module and Catalog's SEO port.
package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	catalogDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/postgres/transaction"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/seo/domain"
)

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

func (r *Repository) Get(ctx context.Context, resourceType string, resourceID uuid.UUID, locale string) (*domain.Metadata, error) {
	var value record
	if err := r.database(ctx).Where("resource_type = ? AND resource_id = ? AND locale = ?", resourceType, resourceID, locale).First(&value).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return value.toDomain(), nil
}

// Upsert joins the caller's transaction when one is in flight. The audited
// admin facade rolls its transaction back when recording the audit entry
// fails; writing through the pool instead would leave the metadata change
// committed with no record of who made it.
func (r *Repository) Upsert(ctx context.Context, command domain.UpsertCommand) (*domain.Metadata, error) {
	value := record{ID: uuid.New(), ResourceType: command.ResourceType, ResourceID: command.ResourceID, Locale: command.Locale, Title: command.Title, Description: command.Description, Keywords: command.Keywords, OGImageRef: command.OGImageRef}
	var saved record
	err := transaction.Within(ctx, r.db, func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "resource_type"}, {Name: "resource_id"}, {Name: "locale"}},
			DoUpdates: clause.AssignmentColumns([]string{"title", "description", "keywords", "og_image_ref", "updated_at"}),
		}).Create(&value).Error; err != nil {
			return err
		}
		// Read back through the same transaction: on conflict the returned row
		// carries the existing id, which a pooled connection would not see.
		return tx.Where("resource_type = ? AND resource_id = ? AND locale = ?", command.ResourceType, command.ResourceID, command.Locale).First(&saved).Error
	})
	if err != nil {
		return nil, err
	}
	return saved.toDomain(), nil
}

func (r *Repository) Delete(ctx context.Context, resourceType string, resourceID uuid.UUID, locale string) error {
	result := r.database(ctx).Where("resource_type = ? AND resource_id = ? AND locale = ?", resourceType, resourceID, locale).Delete(&record{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// SEOForResources performs one indexed lookup for an entire catalog page.
func (r *Repository) SEOForResources(ctx context.Context, resourceType string, resourceIDs []uuid.UUID, locale string) (map[uuid.UUID]catalogDomain.ProductSEO, error) {
	result := make(map[uuid.UUID]catalogDomain.ProductSEO, len(resourceIDs))
	if len(resourceIDs) == 0 {
		return result, nil
	}
	var records []record
	if err := r.database(ctx).Where("resource_type = ? AND resource_id IN ? AND locale = ?", resourceType, resourceIDs, locale).Find(&records).Error; err != nil {
		return nil, err
	}
	for _, value := range records {
		result[value.ResourceID] = catalogDomain.ProductSEO{Title: value.Title, Description: value.Description, Keywords: value.Keywords, OGImageRef: value.OGImageRef}
	}
	return result, nil
}

func (r *Repository) database(ctx context.Context) *gorm.DB {
	if tx, err := transaction.FromContext(ctx); err == nil {
		return tx.WithContext(ctx)
	}
	return r.db.WithContext(ctx)
}

type record struct {
	ID           uuid.UUID `gorm:"column:id;type:uuid;primaryKey"`
	ResourceType string    `gorm:"column:resource_type"`
	ResourceID   uuid.UUID `gorm:"column:resource_id;type:uuid"`
	Locale       string    `gorm:"column:locale"`
	Title        string    `gorm:"column:title"`
	Description  string    `gorm:"column:description"`
	Keywords     string    `gorm:"column:keywords"`
	OGImageRef   string    `gorm:"column:og_image_ref"`
	CreatedAt    int64     `gorm:"-"`
}

func (record) TableName() string { return "seo_metadata" }
func (r record) toDomain() *domain.Metadata {
	return &domain.Metadata{ID: r.ID, ResourceType: r.ResourceType, ResourceID: r.ResourceID, Locale: r.Locale, Title: r.Title, Description: r.Description, Keywords: r.Keywords, OGImageRef: r.OGImageRef}
}

var _ domain.Repository = (*Repository)(nil)
var _ catalogDomain.ProductSEOReader = (*Repository)(nil)
