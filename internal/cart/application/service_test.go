package application

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/cart/domain"
)

func TestAddValidatesOwnerAndItemBeforeRepository(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo)
	if _, err := service.Add(context.Background(), domain.Owner{}, domain.Item{VariantID: uuid.New(), Quantity: 1}); !errors.Is(err, domain.ErrInvalidOwner) {
		t.Fatalf("Add() error = %v, want invalid owner", err)
	}
	if repo.called {
		t.Fatal("repository called for invalid owner")
	}
	owner := domain.Owner{SessionID: ptr(uuid.New())}
	if _, err := service.Add(context.Background(), owner, domain.Item{}); !errors.Is(err, domain.ErrInvalidItem) {
		t.Fatalf("Add() error = %v, want invalid item", err)
	}
	if _, err := service.Add(context.Background(), owner, domain.Item{VariantID: uuid.New(), Quantity: 2}); err != nil || !repo.called {
		t.Fatalf("Add() error = %v, repository called = %v", err, repo.called)
	}
}
func ptr(value uuid.UUID) *uuid.UUID { return &value }

type fakeRepository struct{ called bool }

func (r *fakeRepository) GetOrCreate(context.Context, domain.Owner) (*domain.Cart, error) {
	r.called = true
	return &domain.Cart{}, nil
}
func (r *fakeRepository) Add(context.Context, domain.Owner, domain.Item) (*domain.Cart, error) {
	r.called = true
	return &domain.Cart{}, nil
}
func (r *fakeRepository) SetQuantity(context.Context, domain.Owner, domain.Item) (*domain.Cart, error) {
	r.called = true
	return &domain.Cart{}, nil
}
func (r *fakeRepository) Remove(context.Context, domain.Owner, uuid.UUID) (*domain.Cart, error) {
	r.called = true
	return &domain.Cart{}, nil
}
func (r *fakeRepository) SetPromoCode(_ context.Context, _ domain.Owner, code string) (*domain.Cart, error) {
	r.called = true
	return &domain.Cart{AppliedPromoCode: code}, nil
}
