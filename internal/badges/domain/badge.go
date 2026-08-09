package domain

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidBadge = errors.New("badge is invalid")
	ErrNotFound     = errors.New("badge not found")
)

type Translation struct {
	Locale string
	Name   string
}

type Badge struct {
	ID           uuid.UUID
	Slug         string
	Color        string
	Translations []Translation
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type CreateCommand struct {
	Slug, Color  string
	Translations []Translation
}
type UpdateCommand struct {
	Slug, Color  string
	Translations []Translation
}

type Service interface {
	Get(context.Context, uuid.UUID) (*Badge, error)
	List(context.Context) ([]Badge, error)
	Create(context.Context, CreateCommand) (*Badge, error)
	Update(context.Context, uuid.UUID, UpdateCommand) (*Badge, error)
	Delete(context.Context, uuid.UUID) error
	AssignProduct(context.Context, uuid.UUID, uuid.UUID) error
	RemoveProduct(context.Context, uuid.UUID, uuid.UUID) error
}

type Repository interface {
	Get(context.Context, uuid.UUID) (*Badge, error)
	List(context.Context) ([]Badge, error)
	Create(context.Context, CreateCommand) (*Badge, error)
	Update(context.Context, uuid.UUID, UpdateCommand) (*Badge, error)
	Delete(context.Context, uuid.UUID) error
	AssignProduct(context.Context, uuid.UUID, uuid.UUID) error
	RemoveProduct(context.Context, uuid.UUID, uuid.UUID) error
}
