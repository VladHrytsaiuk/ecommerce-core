package postgres

import (
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	localeDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/core/locale/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/postgres/transaction"
)

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

// Synchronize is transactional so the partial unique index on a default locale
// is never observed in an invalid intermediate state.
func (r *Repository) Synchronize(ctx context.Context, locales []localeDomain.Locale) error {
	return transaction.Within(ctx, r.db, func(tx *gorm.DB) error {
		if err := tx.Model(&localeDomain.Locale{}).Where("is_default = ?", true).Update("is_default", false).Error; err != nil {
			return err
		}
		for _, locale := range locales {
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "code"}},
				DoUpdates: clause.AssignmentColumns([]string{"name", "is_default", "is_active"}),
			}).Create(&locale).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
