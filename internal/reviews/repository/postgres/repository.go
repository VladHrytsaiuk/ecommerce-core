// Package postgres implements Reviews and its local rating projection.
package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	catalogDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/reviews/domain"
)

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

func (repository *Repository) Create(ctx context.Context, command domain.CreateCommand) (*domain.Review, error) {
	record := reviewRecord{ID: uuid.New(), ProductID: command.ProductID, UserID: command.UserID, Rating: command.Rating, Comment: command.Comment, Status: string(domain.StatusPending)}
	if err := repository.db.WithContext(ctx).Create(&record).Error; err != nil {
		if isUniqueViolation(err) {
			return nil, domain.ErrAlreadyExists
		}
		return nil, err
	}
	return record.toDomain(), nil
}

func (repository *Repository) ListApproved(ctx context.Context, productID uuid.UUID) ([]domain.Review, error) {
	var records []reviewRecord
	if err := repository.db.WithContext(ctx).Where("product_id = ? AND status = ?", productID, domain.StatusApproved).Order("created_at DESC").Find(&records).Error; err != nil {
		return nil, err
	}
	reviews := make([]domain.Review, 0, len(records))
	for _, record := range records {
		reviews = append(reviews, *record.toDomain())
	}
	return reviews, nil
}

// SetStatus makes the moderation transition and refreshes the local aggregate
// projection in one transaction. Catalog observes the projection through its
// reader port; no Catalog repository is imported here.
func (repository *Repository) SetStatus(ctx context.Context, reviewID uuid.UUID, status domain.Status) (*domain.Review, error) {
	var updated *domain.Review
	err := repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var record reviewRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&record, "id = ?", reviewID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrNotFound
			}
			return err
		}
		if err := lockProductForRatingProjection(ctx, tx, record.ProductID); err != nil {
			return err
		}
		if err := tx.Model(&record).Updates(map[string]any{"status": string(status), "updated_at": time.Now().UTC()}).Error; err != nil {
			return err
		}
		record.Status = string(status)
		record.UpdatedAt = time.Now().UTC()
		if err := refreshRatingProjection(ctx, tx, record.ProductID); err != nil {
			return err
		}
		updated = record.toDomain()
		return nil
	})
	return updated, err
}

func (repository *Repository) Delete(ctx context.Context, reviewID uuid.UUID) error {
	return repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var record reviewRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&record, "id = ?", reviewID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrNotFound
			}
			return err
		}
		if err := lockProductForRatingProjection(ctx, tx, record.ProductID); err != nil {
			return err
		}
		if err := tx.Delete(&record).Error; err != nil {
			return err
		}
		return refreshRatingProjection(ctx, tx, record.ProductID)
	})
}

func lockProductForRatingProjection(ctx context.Context, tx *gorm.DB, productID uuid.UUID) error {
	var locked struct {
		ID uuid.UUID `gorm:"column:id"`
	}
	if err := tx.WithContext(ctx).Raw(`SELECT id FROM products WHERE id = ? FOR NO KEY UPDATE`, productID).Scan(&locked).Error; err != nil {
		return err
	}
	if locked.ID == uuid.Nil {
		return domain.ErrNotFound
	}
	return nil
}

func (repository *Repository) RatingForProduct(ctx context.Context, productID uuid.UUID) (*catalogDomain.ProductRating, error) {
	var record ratingRecord
	if err := repository.db.WithContext(ctx).First(&record, "product_id = ?", productID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &catalogDomain.ProductRating{ReviewCount: record.ReviewCount, AverageHundredths: record.AverageHundredths}, nil
}

func refreshRatingProjection(ctx context.Context, tx *gorm.DB, productID uuid.UUID) error {
	if err := tx.WithContext(ctx).Where("product_id = ?", productID).Delete(&ratingRecord{}).Error; err != nil {
		return err
	}
	return tx.WithContext(ctx).Exec(`
		INSERT INTO product_review_ratings (product_id, review_count, rating_sum, average_rating_hundredths, updated_at)
		SELECT product_id, COUNT(*), SUM(rating), ROUND(AVG(rating) * 100)::SMALLINT, CURRENT_TIMESTAMP
		FROM reviews
		WHERE product_id = ? AND status = 'approved'
		GROUP BY product_id`, productID).Error
}

func isUniqueViolation(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "unique")
}

type reviewRecord struct {
	ID        uuid.UUID `gorm:"column:id;type:uuid;primaryKey"`
	ProductID uuid.UUID `gorm:"column:product_id;type:uuid"`
	UserID    uuid.UUID `gorm:"column:user_id;type:uuid"`
	Rating    int       `gorm:"column:rating"`
	Comment   string    `gorm:"column:comment"`
	Status    string    `gorm:"column:status"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (reviewRecord) TableName() string { return "reviews" }

func (record reviewRecord) toDomain() *domain.Review {
	return &domain.Review{ID: record.ID, ProductID: record.ProductID, UserID: record.UserID, Rating: record.Rating, Comment: record.Comment, Status: domain.Status(record.Status), CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt}
}

type ratingRecord struct {
	ProductID         uuid.UUID `gorm:"column:product_id;type:uuid;primaryKey"`
	ReviewCount       int       `gorm:"column:review_count"`
	AverageHundredths int       `gorm:"column:average_rating_hundredths"`
}

func (ratingRecord) TableName() string { return "product_review_ratings" }

var _ domain.Repository = (*Repository)(nil)
var _ catalogDomain.ProductRatingReader = (*Repository)(nil)
