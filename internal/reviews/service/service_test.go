package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/reviews/domain"
)

func TestCreateRejectsUnauthorisedOrInvalidReview(t *testing.T) {
	repository := &repositoryFake{}
	service := New(repository)
	if _, err := service.Create(context.Background(), domain.CreateCommand{ProductID: uuid.New(), Rating: 5, Comment: "good"}); !errors.Is(err, domain.ErrInvalidReview) {
		t.Fatalf("Create() error = %v, want ErrInvalidReview", err)
	}
	if repository.createCalls != 0 {
		t.Fatalf("repository Create calls = %d, want 0", repository.createCalls)
	}
}

func TestSetStatusValidatesModerationStatus(t *testing.T) {
	repository := &repositoryFake{}
	service := New(repository)
	if _, err := service.SetStatus(context.Background(), uuid.New(), domain.Status("published")); !errors.Is(err, domain.ErrInvalidReview) {
		t.Fatalf("SetStatus() error = %v, want ErrInvalidReview", err)
	}
}

type repositoryFake struct{ createCalls int }

func (repository *repositoryFake) Create(_ context.Context, command domain.CreateCommand) (*domain.Review, error) {
	repository.createCalls++
	return &domain.Review{ID: uuid.New(), ProductID: command.ProductID, UserID: command.UserID, Rating: command.Rating, Comment: command.Comment, Status: domain.StatusPending}, nil
}
func (*repositoryFake) ListApproved(context.Context, uuid.UUID) ([]domain.Review, error) {
	return nil, nil
}
func (*repositoryFake) SetStatus(context.Context, uuid.UUID, domain.Status) (*domain.Review, error) {
	return nil, nil
}
func (*repositoryFake) Delete(context.Context, uuid.UUID) error { return nil }
