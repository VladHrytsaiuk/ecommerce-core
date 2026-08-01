package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/document/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/db"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/pagination"
	"gorm.io/gorm"
)

type documentRepository struct {
	db *gorm.DB
	l  logger.Logger
}

func NewDocumentRepository(db *gorm.DB, l logger.Logger) domain.DocumentRepository {
	return &documentRepository{db: db, l: l}
}

func (r *documentRepository) getDB(ctx context.Context) *gorm.DB {
	return db.GetTx(ctx, r.db).WithContext(ctx)
}

func (r *documentRepository) Create(ctx context.Context, doc *domain.Document) error {
	err := r.getDB(ctx).Create(doc).Error
	if err != nil {
		if isUniqueViolation(err) {
			return domain.ErrSlugAlreadyExists
		}
		r.l.Errorw("failed to create document", "error", err)
		return err
	}
	return nil
}

func (r *documentRepository) CreateVersion(ctx context.Context, version *domain.DocumentVersion) error {
	err := r.getDB(ctx).Create(version).Error
	if err != nil {
		r.l.Errorw("failed to create document version", "error", err)
		return err
	}
	return nil
}

func (r *documentRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.Document, error) {
	var doc domain.Document
	err := r.getDB(ctx).
		Preload("CurrentVersion").
		Preload("Versions", func(db *gorm.DB) *gorm.DB {
			return db.Order("version_number DESC")
		}).
		Preload("Versions.CreatedByUser").
		Where("id = ?", id).
		First(&doc).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrDocumentNotFound
		}
		r.l.Errorw("failed to find document by id", "error", err, "id", id)
		return nil, err
	}
	return &doc, nil
}

func (r *documentRepository) FindBySlug(ctx context.Context, slug string) (*domain.Document, error) {
	var doc domain.Document
	err := r.getDB(ctx).
		Preload("CurrentVersion").
		Where("slug = ?", slug).
		First(&doc).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrDocumentNotFound
		}
		r.l.Errorw("failed to find document by slug", "error", err, "slug", slug)
		return nil, err
	}
	return &doc, nil
}

func (r *documentRepository) FindAll(ctx context.Context, pgn pagination.Params) ([]domain.Document, int64, error) {
	var docs []domain.Document
	var total int64

	query := r.getDB(ctx).Model(&domain.Document{})

	err := query.Count(&total).Error
	if err != nil {
		r.l.Errorw("failed to count documents", "error", err)
		return nil, 0, err
	}

	offset := (pgn.Page - 1) * pgn.Limit
	err = query.
		Preload("CurrentVersion").
		Order("created_at DESC").
		Offset(offset).
		Limit(pgn.Limit).
		Find(&docs).Error

	if err != nil {
		r.l.Errorw("failed to find documents", "error", err)
		return nil, 0, err
	}

	return docs, total, nil
}

func (r *documentRepository) FindVersionByID(ctx context.Context, id uuid.UUID) (*domain.DocumentVersion, error) {
	var version domain.DocumentVersion
	err := r.getDB(ctx).Where("id = ?", id).First(&version).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrVersionNotFound
		}
		r.l.Errorw("failed to find document version by id", "error", err, "id", id)
		return nil, err
	}
	return &version, nil
}

func (r *documentRepository) GetNextVersionNumber(ctx context.Context, documentID uuid.UUID) (int, error) {
	var maxVersion int
	err := r.getDB(ctx).
		Model(&domain.DocumentVersion{}).
		Where("document_id = ?", documentID).
		Select("COALESCE(MAX(version_number), 0)").
		Scan(&maxVersion).Error

	if err != nil {
		r.l.Errorw("failed to get max version number", "error", err, "document_id", documentID)
		return 0, err
	}
	return maxVersion + 1, nil
}

func (r *documentRepository) Update(ctx context.Context, doc *domain.Document) error {
	err := r.getDB(ctx).Omit("CurrentVersion", "Versions").Save(doc).Error
	if err != nil {
		if isUniqueViolation(err) {
			return domain.ErrSlugAlreadyExists
		}
		r.l.Errorw("failed to update document", "error", err, "id", doc.ID)
		return err
	}
	return nil
}

func (r *documentRepository) Delete(ctx context.Context, id uuid.UUID) error {
	err := r.getDB(ctx).Where("id = ?", id).Delete(&domain.Document{}).Error
	if err != nil {
		r.l.Errorw("failed to delete document", "error", err, "id", id)
		return err
	}
	return nil
}

func (r *documentRepository) Atomic(ctx context.Context, fn func(domain.DocumentRepository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repo := NewDocumentRepository(tx, r.l)
		return fn(repo)
	})
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return true
	}
	return false
}
