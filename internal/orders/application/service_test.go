package application

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/orders/domain"
)

func TestCreateStoresValidatedPendingPaymentSnapshot(t *testing.T) {
	price := mustMoney(1000, "EUR")
	tax := mustMoney(200, "EUR")
	total := mustMoney(1200, "EUR")
	repo := &fakeRepo{}
	order, err := NewService(repo).Create(context.Background(), domain.Draft{Number: "ES-1", Subtotal: price, Tax: tax, Total: total, Items: []domain.Item{{ProductName: "Cream", Quantity: 1, UnitPrice: price, Total: price}}})
	if err != nil || !repo.created || order.Status != domain.StatusPendingPayment {
		t.Fatalf("order=%+v err=%v", order, err)
	}
}

type fakeRepo struct{ created bool }

func (r *fakeRepo) Create(context.Context, *domain.Order) error { r.created = true; return nil }
func (r *fakeRepo) ListByCustomer(context.Context, uuid.UUID, int, int) (domain.Page, error) {
	return domain.Page{}, nil
}
