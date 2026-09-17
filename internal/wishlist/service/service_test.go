package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/wishlist/domain"
)

func TestServiceRejectsInvalidOwnerAndVariant(t *testing.T) {
	repository := &fakeRepository{}
	service := New(repository)

	if err := service.Add(context.Background(), domain.Owner{}, uuid.New()); !errors.Is(err, domain.ErrInvalidOwner) {
		t.Fatalf("Add() error = %v, want ErrInvalidOwner", err)
	}
	userID := uuid.New()
	if err := service.Add(context.Background(), domain.Owner{UserID: &userID}, uuid.Nil); !errors.Is(err, domain.ErrInvalidVariant) {
		t.Fatalf("Add() error = %v, want ErrInvalidVariant", err)
	}
	if repository.addCalls != 0 {
		t.Fatalf("repository Add() calls = %d, want 0", repository.addCalls)
	}
}

func TestServiceMergeGuestWishlistDelegatesToRepository(t *testing.T) {
	repository := &fakeRepository{}
	service := New(repository)
	userID, sessionID := uuid.New(), uuid.New()

	if err := service.MergeGuestWishlist(context.Background(), userID, sessionID); err != nil {
		t.Fatalf("MergeGuestWishlist() error = %v", err)
	}
	if repository.mergeUserID != userID || repository.mergeSessionID != sessionID {
		t.Fatalf("merge arguments = (%s, %s), want (%s, %s)", repository.mergeUserID, repository.mergeSessionID, userID, sessionID)
	}
}

type fakeRepository struct {
	addCalls       int
	mergeUserID    uuid.UUID
	mergeSessionID uuid.UUID
}

func (repository *fakeRepository) List(context.Context, domain.Owner) ([]domain.WishlistItem, error) {
	return nil, nil
}

func (repository *fakeRepository) Add(context.Context, domain.Owner, uuid.UUID) error {
	repository.addCalls++
	return nil
}

func (repository *fakeRepository) Remove(context.Context, domain.Owner, uuid.UUID) error { return nil }

func (repository *fakeRepository) MergeGuestWishlist(_ context.Context, userID, sessionID uuid.UUID) error {
	repository.mergeUserID = userID
	repository.mergeSessionID = sessionID
	return nil
}
