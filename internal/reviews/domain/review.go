// Package domain defines the Reviews module without HTTP, ORM, or Catalog
// repository dependencies.
package domain

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidReview = errors.New("review is invalid")
	ErrNotFound      = errors.New("review not found")
	ErrAlreadyExists = errors.New("review already exists for this product")
)

type Status string

const (
	StatusPending  Status = "pending"
	StatusApproved Status = "approved"
	StatusRejected Status = "rejected"
)

func (status Status) Valid() bool {
	return status == StatusPending || status == StatusApproved || status == StatusRejected
}

type Review struct {
	ID        uuid.UUID
	ProductID uuid.UUID
	UserID    uuid.UUID
	Rating    int
	Comment   string
	Status    Status
	CreatedAt time.Time
	UpdatedAt time.Time
}

type CreateCommand struct {
	ProductID uuid.UUID
	UserID    uuid.UUID
	Rating    int
	Comment   string
}

type ReviewService interface {
	Create(context.Context, CreateCommand) (*Review, error)
	ListApproved(context.Context, uuid.UUID) ([]Review, error)
	SetStatus(context.Context, uuid.UUID, Status) (*Review, error)
	Delete(context.Context, uuid.UUID) error
}

type ReviewRepository interface {
	Create(context.Context, CreateCommand) (*Review, error)
	ListApproved(context.Context, uuid.UUID) ([]Review, error)
	SetStatus(context.Context, uuid.UUID, Status) (*Review, error)
	Delete(context.Context, uuid.UUID) error
}

type Service = ReviewService
type Repository = ReviewRepository
