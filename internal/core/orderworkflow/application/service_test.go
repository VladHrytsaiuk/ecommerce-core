package application

import (
	"context"
	"testing"
	"time"

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
	confirmation := paymentConfirmation(orderID)
	if err := service.RegisterPayment(context.Background(), confirmation.PaymentAttempt); err != nil {
		t.Fatalf("RegisterPayment() error = %v", err)
	}
	if err := service.MarkPaid(context.Background(), confirmation); err != nil {
		t.Fatalf("MarkPaid() error = %v", err)
	}
	if err := service.CancelPending(context.Background(), orderID); err != nil {
		t.Fatalf("CancelPending() error = %v", err)
	}
	if repo.paid.OrderID != orderID || repo.registered.OrderID != orderID || repo.cancelled != orderID {
		t.Fatalf("workflow transitions = %+v", repo)
	}
}

func paymentConfirmation(orderID uuid.UUID) workflowDomain.PaymentConfirmation {
	amount, _ := money.New(1000, "EUR")
	return workflowDomain.PaymentConfirmation{PaymentAttempt: workflowDomain.PaymentAttempt{OrderID: orderID, Provider: "fake", ProviderReference: "payment-1", Amount: amount}, Status: "paid"}
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
	paid           workflowDomain.PaymentConfirmation
	registered     workflowDomain.PaymentAttempt
}

func (*fakeRepository) RecordCheckoutAttempt(context.Context, workflowDomain.CheckoutAttemptRequest) error {
	return nil
}
func (*fakeRepository) FindCheckoutAttempt(context.Context, string) (*workflowDomain.CheckoutAttempt, error) {
	return nil, nil
}
func (*fakeRepository) ClaimPendingCheckoutAttempt(context.Context, time.Duration, time.Duration) (*workflowDomain.CheckoutAttempt, error) {
	return nil, nil
}
func (*fakeRepository) MarkCheckoutAttemptFailed(context.Context, uuid.UUID) error   { return nil }
func (*fakeRepository) RetryCheckoutAttempt(context.Context, uuid.UUID, error) error { return nil }

func (r *fakeRepository) RegisterPayment(_ context.Context, attempt workflowDomain.PaymentAttempt) error {
	r.registered = attempt
	return nil
}

func (r *fakeRepository) CreatePending(_ context.Context, order *ordersDomain.Order, reservationIDs []uuid.UUID) error {
	r.created = order
	r.reservationIDs = reservationIDs
	return nil
}
func (r *fakeRepository) CreatePendingCheckout(ctx context.Context, order *ordersDomain.Order, reservationIDs []uuid.UUID, _ workflowDomain.CheckoutAttemptRequest) error {
	return r.CreatePending(ctx, order, reservationIDs)
}
func (r *fakeRepository) CreatePaidCheckout(ctx context.Context, order *ordersDomain.Order, reservationIDs []uuid.UUID, _ workflowDomain.CheckoutAttemptRequest) error {
	return r.CreatePending(ctx, order, reservationIDs)
}
func (*fakeRepository) ExpirePendingCheckout(context.Context, time.Time) (bool, error) {
	return false, nil
}
func (r *fakeRepository) CancelPending(_ context.Context, orderID uuid.UUID) error {
	r.cancelled = orderID
	return nil
}
func (r *fakeRepository) MarkPaid(_ context.Context, confirmation workflowDomain.PaymentConfirmation) error {
	r.paid = confirmation
	return nil
}
func (*fakeRepository) MarkFailed(context.Context, workflowDomain.PaymentConfirmation) error {
	return nil
}
func (*fakeRepository) MarkRefunded(context.Context, workflowDomain.PaymentConfirmation) error { return nil }

var _ workflowDomain.Repository = (*fakeRepository)(nil)
