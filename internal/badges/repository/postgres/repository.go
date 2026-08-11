// Package postgres implements the optional Badges module and Catalog's badge port.
package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/badges/domain"
	catalogDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/postgres/transaction"
)

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

func (r *Repository) Get(ctx context.Context, id uuid.UUID) (*domain.Badge, error) {
	return r.find(ctx, id)
}
func (r *Repository) List(ctx context.Context) ([]domain.Badge, error) {
	var records []badgeRecord
	if err := r.db.WithContext(ctx).Preload("Translations").Order("slug ASC").Find(&records).Error; err != nil {
		return nil, err
	}
	result := make([]domain.Badge, 0, len(records))
	for _, value := range records {
		result = append(result, *value.toDomain())
	}
	return result, nil
}

func (r *Repository) Create(ctx context.Context, command domain.CreateCommand) (*domain.Badge, error) {
	record := badgeRecord{ID: uuid.New(), Slug: command.Slug, Color: command.Color}
	err := transaction.Within(ctx, r.db, func(tx *gorm.DB) error {
		if err := tx.Create(&record).Error; err != nil {
			return err
		}
		return replaceTranslations(tx, record.ID, command.Translations)
	})
	if err != nil {
		return nil, err
	}
	return r.find(ctx, record.ID)
}
func (r *Repository) Update(ctx context.Context, id uuid.UUID, command domain.UpdateCommand) (*domain.Badge, error) {
	err := transaction.Within(ctx, r.db, func(tx *gorm.DB) error {
		result := tx.Model(&badgeRecord{}).Where("id = ?", id).Updates(map[string]any{"slug": command.Slug, "color": command.Color})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return domain.ErrNotFound
		}
		return replaceTranslations(tx, id, command.Translations)
	})
	if err != nil {
		return nil, err
	}
	return r.find(ctx, id)
}
func (r *Repository) Delete(ctx context.Context, id uuid.UUID) error {
	result := r.db.WithContext(ctx).Delete(&badgeRecord{}, "id = ?", id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}
func (r *Repository) AssignProduct(ctx context.Context, badgeID, productID uuid.UUID) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&productBadgeRecord{BadgeID: badgeID, ProductID: productID}).Error
}
func (r *Repository) RemoveProduct(ctx context.Context, badgeID, productID uuid.UUID) error {
	result := r.db.WithContext(ctx).Where("badge_id = ? AND product_id = ?", badgeID, productID).Delete(&productBadgeRecord{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}
func (r *Repository) find(ctx context.Context, id uuid.UUID) (*domain.Badge, error) {
	var record badgeRecord
	if err := r.db.WithContext(ctx).Preload("Translations").First(&record, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return record.toDomain(), nil
}

// BadgesForProducts is the page-level bulk read used by Catalog, not a loop
// of product lookups. It is one joined SQL query over module-owned tables.
func (r *Repository) BadgesForProducts(ctx context.Context, productIDs []uuid.UUID, locale string) (map[uuid.UUID][]catalogDomain.ProductBadge, error) {
	result := make(map[uuid.UUID][]catalogDomain.ProductBadge, len(productIDs))
	if len(productIDs) == 0 {
		return result, nil
	}
	type row struct {
		ProductID, BadgeID uuid.UUID
		Slug, Color, Name  string
	}
	var rows []row
	err := r.db.WithContext(ctx).Table("product_badges pb").Select("pb.product_id, b.id AS badge_id, b.slug, b.color, bt.name").Joins("JOIN badges b ON b.id = pb.badge_id").Joins("JOIN badge_translations bt ON bt.badge_id = b.id AND bt.locale = ?", locale).Where("pb.product_id IN ?", productIDs).Order("b.slug ASC").Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, value := range rows {
		result[value.ProductID] = append(result[value.ProductID], catalogDomain.ProductBadge{ID: value.BadgeID, Slug: value.Slug, Color: value.Color, Name: value.Name})
	}
	return result, nil
}
func replaceTranslations(tx *gorm.DB, badgeID uuid.UUID, translations []domain.Translation) error {
	if err := tx.Where("badge_id = ?", badgeID).Delete(&translationRecord{}).Error; err != nil {
		return err
	}
	records := make([]translationRecord, 0, len(translations))
	for _, t := range translations {
		records = append(records, translationRecord{BadgeID: badgeID, Locale: t.Locale, Name: t.Name})
	}
	if len(records) == 0 {
		return nil
	}
	return tx.Create(&records).Error
}

type badgeRecord struct {
	ID           uuid.UUID           `gorm:"column:id;type:uuid;primaryKey"`
	Slug         string              `gorm:"column:slug"`
	Color        string              `gorm:"column:color"`
	Translations []translationRecord `gorm:"foreignKey:BadgeID"`
}

func (badgeRecord) TableName() string { return "badges" }
func (r badgeRecord) toDomain() *domain.Badge {
	translations := make([]domain.Translation, 0, len(r.Translations))
	for _, t := range r.Translations {
		translations = append(translations, domain.Translation{Locale: t.Locale, Name: t.Name})
	}
	return &domain.Badge{ID: r.ID, Slug: r.Slug, Color: r.Color, Translations: translations}
}

type translationRecord struct {
	BadgeID uuid.UUID `gorm:"column:badge_id;type:uuid;primaryKey"`
	Locale  string    `gorm:"column:locale;primaryKey"`
	Name    string    `gorm:"column:name"`
}

func (translationRecord) TableName() string { return "badge_translations" }

type productBadgeRecord struct {
	ProductID uuid.UUID `gorm:"column:product_id;type:uuid;primaryKey"`
	BadgeID   uuid.UUID `gorm:"column:badge_id;type:uuid;primaryKey"`
}

func (productBadgeRecord) TableName() string { return "product_badges" }

var _ domain.Repository = (*Repository)(nil)
var _ catalogDomain.ProductBadgeReader = (*Repository)(nil)
