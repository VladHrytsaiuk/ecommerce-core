package application

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
	workflowDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/core/orderworkflow/domain"
	ordersDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/orders/domain"
)

func TestCreatePendingBuildsValidatedOrderAndAssociatesReservations(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo)
	reservationID := uuid.New()
	order, err := service.CreatePending(context.Background(), validDraft(t), []uuid.UUID{reservationID})
	if err != nil {
		t.Fatalf("CreatePending() error = %v", err)
	}
	if repo.created == nil || repo.created.ID != order.ID || len(repo.reservationIDs) != 1 || repo.reservationIDs[0] != reservationID {
		t.Fatalf("workflow repository call = %+v, %+v", repo.created, repo.reservationIDs)
	}
}

func TestCreatePendingRejectsDuplicateReservationID(t *testing.T) {
	reservationID := uuid.New()
	_, err := NewService(&fakeRepository{}).CreatePending(context.Background(), validDraft(t), []uuid.UUID{reservationID, reservationID})
	if err == nil {
		t.Fatal("CreatePending() error = nil, want duplicate reservation validation")
	}
}

func TestTransitionDelegatesByOrderID(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo)
	orderID := uuid.New()
	if err := service.MarkPaid(context.Background(), orderID); err != nil {
		t.Fatalf("MarkPaid() error = %v", err)
	}
	if err := service.CancelPending(context.Background(), orderID); err != nil {
		t.Fatalf("CancelPending() error = %v", err)
	}
	if repo.paid != orderID || repo.cancelled != orderID {
		t.Fatalf("workflow transitions = %+v", repo)
	}
}

func validDraft(t *testing.T) ordersDomain.Draft {
	t.Helper()
	price, err := money.New(1000, "EUR")
	if err != nil {
		t.Fatal(err)
	}
	return ordersDomain.Draft{Number: "ES-100", Subtotal: price, Tax: money.Money{Amount: 0, Currency: "EUR"}, Total: price, Items: []ordersDomain.Item{{ProductName: "Cream", Quantity: 1, UnitPrice: price, Total: price}}}
}

type fakeRepository struct {
	created        *ordersDomain.Order
	reservationIDs []uuid.UUID
	cancelled      uuid.UUID
	paid           uuid.UUID
}

func (r *fakeRepository) CreatePending(_ context.Context, order *ordersDomain.Order, reservationIDs []uuid.UUID) error {
	r.created = order
	r.reservationIDs = reservationIDs
	return nil
}
func (r *fakeRepository) CancelPending(_ context.Context, orderID uuid.UUID) error {
	r.cancelled = orderID
	return nil
}
func (r *fakeRepository) MarkPaid(_ context.Context, orderID uuid.UUID) error {
	r.paid = orderID
	return nil
}

var _ workflowDomain.Repository = (*fakeRepository)(nil)
