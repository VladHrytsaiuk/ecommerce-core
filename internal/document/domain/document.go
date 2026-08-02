//go:build legacy
// +build legacy

package domain

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	productDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/product/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/pagination"
	userDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/user/domain"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	ErrDocumentNotFound  = errors.New("document not found")
	ErrVersionNotFound   = errors.New("document version not found")
	ErrSlugAlreadyExists = errors.New("document with this slug already exists")
)

type Document struct {
	ID               uuid.UUID                  `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	Slug             string                     `gorm:"type:varchar(100);unique;not null" json:"slug"`
	Title            productDomain.LocalizedMap `gorm:"type:jsonb;not null" json:"title"`
	CurrentVersionID *uuid.UUID                 `gorm:"type:uuid" json:"current_version_id"`
	CreatedAt        time.Time                  `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt        time.Time                  `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
	DeletedAt        gorm.DeletedAt             `gorm:"index" json:"-"`

	CurrentVersion *DocumentVersion  `gorm:"foreignKey:CurrentVersionID" json:"current_version,omitempty"`
	Versions       []DocumentVersion `gorm:"foreignKey:DocumentID;constraint:OnDelete:CASCADE;" json:"versions,omitempty"`
}

func (Document) TableName() string {
	return "documents"
}

type DocumentVersion struct {
	ID            uuid.UUID       `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	DocumentID    uuid.UUID       `gorm:"type:uuid;not null" json:"document_id"`
	VersionNumber int             `gorm:"not null" json:"version_number"`
	Content       json.RawMessage `gorm:"type:jsonb;not null" json:"content"`
	CreatedBy     uuid.UUID       `gorm:"type:uuid;not null" json:"created_by"`
	Changelog     *string         `gorm:"type:text" json:"changelog"`
	CreatedAt     time.Time       `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`

	CreatedByUser *userDomain.User `gorm:"foreignKey:CreatedBy" json:"created_by_user,omitempty"`
}

func (DocumentVersion) TableName() string {
	return "document_versions"
}

type DocumentRepository interface {
	Create(ctx context.Context, doc *Document) error
	CreateVersion(ctx context.Context, version *DocumentVersion) error
	FindByID(ctx context.Context, id uuid.UUID) (*Document, error)
	FindBySlug(ctx context.Context, slug string) (*Document, error)
	FindAll(ctx context.Context, pgn pagination.Params) ([]Document, int64, error)
	FindVersionByID(ctx context.Context, id uuid.UUID) (*DocumentVersion, error)
	GetNextVersionNumber(ctx context.Context, documentID uuid.UUID) (int, error)
	Update(ctx context.Context, doc *Document) error
	Delete(ctx context.Context, id uuid.UUID) error
	Atomic(ctx context.Context, fn func(DocumentRepository) error) error
}

type DocumentService interface {
	GetBySlug(ctx context.Context, slug string) (*Document, error)
	GetByID(ctx context.Context, id uuid.UUID) (*Document, error)
	GetList(ctx context.Context, pgn pagination.Params) ([]Document, pagination.Metadata, error)
	Create(ctx context.Context, doc *Document, initialContent json.RawMessage, changelog string, createdBy uuid.UUID) error
	SaveVersion(ctx context.Context, documentID uuid.UUID, content json.RawMessage, changelog string, createdBy uuid.UUID) error
	SetActiveVersion(ctx context.Context, documentID uuid.UUID, versionID uuid.UUID) error
	Delete(ctx context.Context, id uuid.UUID) error
}
