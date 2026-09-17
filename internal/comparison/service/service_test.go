package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/comparison/domain"
)

func TestServicePassesConfiguredLimitToRepository(t *testing.T) {
	repository := &repositoryFake{}
	service := New(repository, 4)
	userID := uuid.New()
	variantID := uuid.New()

	if err := service.Add(context.Background(), domain.Owner{UserID: &userID}, variantID); err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if repository.addMaxItems != 4 {
		t.Fatalf("Add() maxItems = %d, want 4", repository.addMaxItems)
	}
}

func TestServiceRejectsInvalidInputBeforeRepository(t *testing.T) {
	repository := &repositoryFake{}
	service := New(repository, 4)

	if err := service.Add(context.Background(), domain.Owner{}, uuid.New()); !errors.Is(err, domain.ErrInvalidOwner) {
		t.Fatalf("Add() error = %v, want ErrInvalidOwner", err)
	}
	userID := uuid.New()
	if err := service.Add(context.Background(), domain.Owner{UserID: &userID}, uuid.Nil); !errors.Is(err, domain.ErrInvalidVariant) {
		t.Fatalf("Add() error = %v, want ErrInvalidVariant", err)
	}
	if repository.addCalls != 0 {
		t.Fatalf("repository Add calls = %d, want 0", repository.addCalls)
	}
}

type repositoryFake struct {
	addCalls    int
	addMaxItems int
}

func (*repositoryFake) List(context.Context, domain.Owner) ([]domain.List, error) { return nil, nil }
func (repository *repositoryFake) Add(_ context.Context, _ domain.Owner, _ uuid.UUID, maxItems int) error {
	repository.addCalls++
	repository.addMaxItems = maxItems
	return nil
}
func (*repositoryFake) Remove(context.Context, domain.Owner, uuid.UUID) error { return nil }
func (*repositoryFake) MergeGuestComparison(context.Context, uuid.UUID, uuid.UUID, int) error {
	return nil
}
