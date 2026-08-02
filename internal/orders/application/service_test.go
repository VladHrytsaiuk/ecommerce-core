package application

import (
	"context"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/orders/domain"
	"testing"
)

func TestCreateStoresValidatedPendingPaymentSnapshot(t *testing.T) {
	price, _ := money.New(1000, "EUR")
	tax, _ := money.New(200, "EUR")
	total, _ := money.New(1200, "EUR")
	repo := &fakeRepo{}
	order, err := NewService(repo).Create(context.Background(), domain.Draft{Number: "ES-1", Subtotal: price, Tax: tax, Total: total, Items: []domain.Item{{ProductName: "Cream", Quantity: 1, UnitPrice: price, Total: price}}})
	if err != nil || !repo.created || order.Status != domain.StatusPendingPayment {
		t.Fatalf("order=%+v err=%v", order, err)
	}
}

type fakeRepo struct{ created bool }

func (r *fakeRepo) Create(context.Context, *domain.Order) error { r.created = true; return nil }
