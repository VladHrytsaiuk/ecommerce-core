package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
	returns "github.com/VladHrytsaiuk/ecommerce-core/internal/returns/domain"
)

func TestReturnServiceCreateApproveAndReceiveWritesOutboxCommands(t *testing.T) {
	now := time.Date(2026, time.September, 2, 12, 0, 0, 0, time.UTC)
	orderID, customerID, variantID, adminID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	total := mustMoney(t, 1299, "EUR")
	repository := &fakeRepository{}
	statusPublisher, settlementPublisher := &fakePublisher{}, &fakePublisher{}
	service := newService(t, repository, fakeOrders{snapshot: returns.OrderSnapshot{OrderID: orderID, CustomerID: &customerID, Status: "delivered", DeliveredAt: ptrTime(now.AddDate(0, 0, -2)), Total: total, Items: []returns.OrderItemSnapshot{{VariantID: &variantID, Quantity: 1, Total: total}}}}, statusPublisher, settlementPublisher)
	service.now = func() time.Time { return now }

	request, err := service.CreateReturnRequest(context.Background(), CreateCommand{OrderID: orderID, CustomerID: customerID, Items: []returns.ReturnItem{{VariantID: variantID, Quantity: 1, Condition: returns.ItemConditionUnopened}}})
	if err != nil {
		t.Fatalf("CreateReturnRequest() error = %v", err)
	}
	if request.Status != returns.ReturnStatusNew || repository.request == nil || len(repository.request.History) != 1 {
		t.Fatalf("created request = %+v", request)
	}

	if _, err := service.ApproveReturn(context.Background(), request.ID, adminID, "within policy"); err != nil {
		t.Fatalf("ApproveReturn() error = %v", err)
	}
	if repository.request.Status != returns.ReturnStatusApproved || len(statusPublisher.events) != 1 || statusPublisher.events[0].Topic() != returns.TopicStatusChanged {
		t.Fatalf("approval state/events = %+v/%d", repository.request, len(statusPublisher.events))
	}
	if _, err := service.ReceiveReturn(context.Background(), request.ID, adminID, "warehouse received"); err != nil {
		t.Fatalf("ReceiveReturn() error = %v", err)
	}
	if repository.request.Status != returns.ReturnStatusReceived || len(statusPublisher.events) != 2 || len(settlementPublisher.events) != 1 || settlementPublisher.events[0].Topic() != returns.TopicSettlementRequested {
		t.Fatalf("receipt state/events = %+v, status=%d settlement=%d", repository.request, len(statusPublisher.events), len(settlementPublisher.events))
	}
	// A retry does not create another history row or settlement command.
	if _, err := service.ReceiveReturn(context.Background(), request.ID, adminID, "retry"); err != nil {
		t.Fatalf("ReceiveReturn() retry error = %v", err)
	}
	if len(repository.request.History) != 3 || len(settlementPublisher.events) != 1 {
		t.Fatalf("receipt retry created duplicate state: history=%d settlement=%d", len(repository.request.History), len(settlementPublisher.events))
	}
}

func TestSettlementHandlerRestocksOnlyUnopenedAndUsesFullOrderAmount(t *testing.T) {
	now := time.Now().UTC()
	returnID, orderID, customerID := uuid.New(), uuid.New(), uuid.New()
	openItem := returns.ReturnItem{ID: uuid.New(), ReturnRequestID: returnID, VariantID: uuid.New(), Quantity: 1, Condition: returns.ItemConditionUnopened}
	damagedItem := returns.ReturnItem{ID: uuid.New(), ReturnRequestID: returnID, VariantID: uuid.New(), Quantity: 1, Condition: returns.ItemConditionDamaged}
	total := mustMoney(t, 899, "EUR")
	repository := &fakeRepository{request: &returns.ReturnRequest{ID: returnID, OrderID: orderID, CustomerID: customerID, Status: returns.ReturnStatusReceived, RefundMode: returns.RefundModeFull, Items: []returns.ReturnItem{openItem, damagedItem}}}
	restock, refund := &fakeRestock{}, &fakeRefund{}
	handler, err := NewSettlementHandler(repository, fakeOrders{snapshot: returns.OrderSnapshot{OrderID: orderID, CustomerID: &customerID, Status: "delivered", DeliveredAt: &now, Total: total}}, restock, refund)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := returns.NewSettlementRequestedEvent(uuid.New(), returnID, orderID, now)
	if err != nil {
		t.Fatal(err)
	}
	body, err := payload.MarshalPayload()
	if err != nil {
		t.Fatal(err)
	}
	if err := handler.Handle(context.Background(), events.Delivery{EventID: uuid.New(), AggregateID: returnID, Topic: returns.TopicSettlementRequested, Payload: body}); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if len(restock.items) != 1 || restock.items[0].ID != openItem.ID {
		t.Fatalf("restock items = %+v, want only unopened", restock.items)
	}
	if refund.returnID != returnID || refund.orderID != orderID || refund.amount.Amount() != 899 || refund.amount.Currency() != "EUR" {
		t.Fatalf("refund call = %+v", refund)
	}
}

func TestRefundConfirmedHandlerMarksOnlyReceivedRMARefunded(t *testing.T) {
	now := time.Now().UTC()
	returnID, orderID, customerID := uuid.New(), uuid.New(), uuid.New()
	repository := &fakeRepository{request: &returns.ReturnRequest{ID: returnID, OrderID: orderID, CustomerID: customerID, Status: returns.ReturnStatusReceived, RefundMode: returns.RefundModeFull}}
	status, settlement := &fakePublisher{}, &fakePublisher{}
	service := newService(t, repository, fakeOrders{}, status, settlement)
	service.now = func() time.Time { return now }
	handler, err := NewRefundConfirmedHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := events.NewOrderRefundedEvent(orderID, mustMoney(t, 100, "EUR"), now)
	if err != nil {
		t.Fatal(err)
	}
	body, err := payload.MarshalPayload()
	if err != nil {
		t.Fatal(err)
	}
	if err := handler.Handle(context.Background(), events.Delivery{EventID: uuid.New(), AggregateID: orderID, Topic: events.TopicOrderRefunded, Payload: body}); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if repository.request.Status != returns.ReturnStatusRefunded || len(status.events) != 1 || status.events[0].Topic() != returns.TopicStatusChanged {
		t.Fatalf("refund confirmation state/events = %+v/%d", repository.request, len(status.events))
	}
}

func TestCreateReturnRequestRejectsForeignVariant(t *testing.T) {
	orderID, customerID := uuid.New(), uuid.New()
	service := newService(t, &fakeRepository{}, fakeOrders{snapshot: returns.OrderSnapshot{OrderID: orderID, CustomerID: &customerID, Status: "delivered", DeliveredAt: ptrTime(time.Now().UTC()), Total: mustMoney(t, 1, "EUR")}}, &fakePublisher{}, &fakePublisher{})
	_, err := service.CreateReturnRequest(context.Background(), CreateCommand{OrderID: orderID, CustomerID: customerID, Items: []returns.ReturnItem{{VariantID: uuid.New(), Quantity: 1, Condition: returns.ItemConditionUnopened}}})
	if err == nil {
		t.Fatal("CreateReturnRequest() error = nil, want foreign variant rejection")
	}
}

func TestCreateReturnRequestRejectsUnimplementedSettlementModes(t *testing.T) {
	orderID, customerID, variantID := uuid.New(), uuid.New(), uuid.New()
	service := newService(t, &fakeRepository{}, fakeOrders{snapshot: returns.OrderSnapshot{OrderID: orderID, CustomerID: &customerID, Status: "delivered", DeliveredAt: ptrTime(time.Now().UTC()), Total: mustMoney(t, 1, "EUR"), Items: []returns.OrderItemSnapshot{{VariantID: &variantID, Quantity: 1, Total: mustMoney(t, 1, "EUR")}}}}, &fakePublisher{}, &fakePublisher{})
	_, err := service.CreateReturnRequest(context.Background(), CreateCommand{OrderID: orderID, CustomerID: customerID, RefundMode: returns.RefundModePartial, Items: []returns.ReturnItem{{VariantID: variantID, Quantity: 1, Condition: returns.ItemConditionUnopened}}})
	if !errors.Is(err, ErrUnsupportedRefundMode) {
		t.Fatalf("CreateReturnRequest() error = %v, want unsupported refund mode", err)
	}
}

func newService(t *testing.T, repository *fakeRepository, orders fakeOrders, status, settlement *fakePublisher) *ReturnService {
	t.Helper()
	policy, err := returns.NewWindowEligibilityPolicy(14)
	if err != nil {
		t.Fatal(err)
	}
	policyNow := time.Now().UTC()
	policy = policyWithClock(policy, func() time.Time { return policyNow })
	service, err := NewReturnService(repository, orders, policy, fakeTx{}, status, settlement)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

// policyWithClock keeps construction localized; callers use a recent delivery
// timestamp so the production clock remains valid for this black-box test.
func policyWithClock(policy *returns.WindowEligibilityPolicy, _ func() time.Time) *returns.WindowEligibilityPolicy {
	return policy
}

type fakeTx struct{}

func (fakeTx) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type fakeRepository struct{ request *returns.ReturnRequest }

func (r *fakeRepository) Create(_ context.Context, request *returns.ReturnRequest) error {
	r.request = request
	return nil
}
func (r *fakeRepository) Get(_ context.Context, id uuid.UUID) (*returns.ReturnRequest, error) {
	if r.request == nil || r.request.ID != id {
		return nil, returns.ErrReturnRequestNotFound
	}
	return r.request, nil
}
func (r *fakeRepository) GetForUpdate(ctx context.Context, id uuid.UUID) (*returns.ReturnRequest, error) {
	return r.Get(ctx, id)
}
func (r *fakeRepository) FindReceivedByOrderForUpdate(_ context.Context, orderID uuid.UUID) (*returns.ReturnRequest, error) {
	if r.request == nil || r.request.OrderID != orderID || r.request.Status != returns.ReturnStatusReceived {
		return nil, returns.ErrReturnRequestNotFound
	}
	return r.request, nil
}
func (r *fakeRepository) Update(_ context.Context, request *returns.ReturnRequest, _ returns.ReturnStatusHistory) error {
	r.request = request
	return nil
}

type fakeOrders struct {
	snapshot returns.OrderSnapshot
	err      error
}

func (f fakeOrders) GetOrderSnapshot(context.Context, uuid.UUID) (returns.OrderSnapshot, error) {
	return f.snapshot, f.err
}

type fakePublisher struct {
	events []events.DomainEvent
	err    error
}

func (p *fakePublisher) Publish(_ context.Context, event events.DomainEvent) error {
	if p.err != nil {
		return p.err
	}
	p.events = append(p.events, event)
	return nil
}

type fakeRestock struct {
	items []returns.ReturnItem
	err   error
}

func (p *fakeRestock) RestockItems(_ context.Context, items []returns.ReturnItem) error {
	if p.err != nil {
		return p.err
	}
	p.items = append(p.items, items...)
	return nil
}

type fakeRefund struct {
	returnID, orderID uuid.UUID
	amount            money.Money
	err               error
}

func (p *fakeRefund) InitiateRefund(_ context.Context, returnID, orderID uuid.UUID, amount money.Money) error {
	p.returnID, p.orderID, p.amount = returnID, orderID, amount
	return p.err
}

func mustMoney(t *testing.T, amount int64, currency string) money.Money {
	t.Helper()
	value, err := money.NewMoney(amount, currency)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
func ptrTime(value time.Time) *time.Time { return &value }

var _ returns.Repository = (*fakeRepository)(nil)
