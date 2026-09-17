package domain

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

const ResourceProduct = "product"

var (
	ErrInvalidMetadata = errors.New("SEO metadata is invalid")
	ErrNotFound        = errors.New("SEO metadata not found")
)

// Metadata is localized SEO data owned entirely by the optional SEO module.
type Metadata struct {
	ID           uuid.UUID
	ResourceType string
	ResourceID   uuid.UUID
	Locale       string
	Title        string
	Description  string
	Keywords     string
	OGImageRef   string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type UpsertCommand struct {
	ResourceType string
	ResourceID   uuid.UUID
	Locale       string
	Title        string
	Description  string
	Keywords     string
	OGImageRef   string
}

type Service interface {
	Get(context.Context, string, uuid.UUID, string) (*Metadata, error)
	Upsert(context.Context, UpsertCommand) (*Metadata, error)
	Delete(context.Context, string, uuid.UUID, string) error
}

type Repository interface {
	Get(context.Context, string, uuid.UUID, string) (*Metadata, error)
	Upsert(context.Context, UpsertCommand) (*Metadata, error)
	Delete(context.Context, string, uuid.UUID, string) error
}
