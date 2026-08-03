package application

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	catalogDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/domain"
	checkoutDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/checkout/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
	workflowDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/core/orderworkflow/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/tax"
	inventoryDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/inventory/domain"
	ordersDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/orders/domain"
	paymentsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/payments/domain"
	"github.com/google/uuid"
)

func TestPreparePaymentAggregatesDuplicateLinesIntoOneAtomicBatch(t *testing.T) {
	inventory := &fakeInventory{}
	price, _ := money.New(1000, "EUR")
	service := NewService(inventory, &fakeVariantFinder{price: price}, mustTaxPolicy(t, tax.ModeVATExcluded, 20), checkoutDomain.Policy{AllowGuest: true}, nil, nil)
	variant, warehouse := uuid.New(), uuid.New()
	prepared, err := service.PreparePayment(context.Background(), checkoutDomain.PrepareRequest{CheckoutID: uuid.New(), Locale: "es", ExpiresAt: time.Now().Add(time.Minute), Lines: []checkoutDomain.Line{{VariantID: variant, WarehouseID: warehouse, Quantity: 1}, {VariantID: variant, WarehouseID: warehouse, Quantity: 2}}})
	if err != nil {
		t.Fatalf("PreparePayment() error = %v", err)
	}
	if len(inventory.batch) != 1 || inventory.batch[0].Quantity != 3 || len(prepared.ReservationIDs) != 1 || len(prepared.Items) != 1 || prepared.Subtotal.Amount != 3000 || prepared.Tax.Amount != 600 || prepared.Total.Amount != 3600 {
		t.Fatalf("batch = %+v, prepared = %+v", inventory.batch, prepared)
	}
}

func TestPreparePaymentDoesNotReserveWhenCatalogVariantIsUnavailable(t *testing.T) {
	inventory := &fakeInventory{}
	price, _ := money.New(1000, "EUR")
	service := NewService(inventory, &fakeVariantFinder{price: price, err: errors.New("variant unavailable")}, mustTaxPolicy(t, tax.ModeNone, 0), checkoutDomain.Policy{AllowGuest: true}, nil, nil)

	_, err := service.PreparePayment(context.Background(), checkoutDomain.PrepareRequest{CheckoutID: uuid.New(), Locale: "es", ExpiresAt: time.Now().Add(time.Minute), Lines: []checkoutDomain.Line{{VariantID: uuid.New(), WarehouseID: uuid.New(), Quantity: 1}}})
	if err == nil {
		t.Fatal("PreparePayment() error = nil, want catalog error")
	}
	if len(inventory.batch) != 0 {
		t.Fatalf("ReserveBatch() was called before catalog validation: %+v", inventory.batch)
	}
}

func TestStartPaymentCreatesOrderBeforeGatewayCall(t *testing.T) {
	inventory := &fakeInventory{}
	price, _ := money.New(1000, "EUR")
	workflow := &fakeWorkflow{}
	gateway := &fakeGateway{}
	service := NewService(inventory, &fakeVariantFinder{price: price}, mustTaxPolicy(t, tax.ModeNone, 0), checkoutDomain.Policy{AllowGuest: true}, workflow, gateway)
	checkoutID := uuid.New()

	started, err := service.StartPayment(context.Background(), checkoutDomain.StartPaymentRequest{Preparation: checkoutDomain.PrepareRequest{CheckoutID: checkoutID, Locale: "es", ExpiresAt: time.Now().Add(time.Minute), Lines: []checkoutDomain.Line{{VariantID: uuid.New(), WarehouseID: uuid.New(), Quantity: 1}}}, OrderNumber: "ES-200", ReturnURL: "https://store.example/return"})
	if err != nil {
		t.Fatalf("StartPayment() error = %v", err)
	}
	if workflow.created == nil || gateway.payment.OrderID != workflow.created.ID || gateway.payment.IdempotencyKey != checkoutID.String() || started.Session.ProviderReference == "" {
		t.Fatalf("workflow/gateway state = %+v / %+v", workflow, gateway)
	}
}

func TestStartPaymentCancelsOrderWhenGatewayFails(t *testing.T) {
	inventory := &fakeInventory{}
	price, _ := money.New(1000, "EUR")
	workflow := &fakeWorkflow{}
	service := NewService(inventory, &fakeVariantFinder{price: price}, mustTaxPolicy(t, tax.ModeNone, 0), checkoutDomain.Policy{AllowGuest: true}, workflow, &fakeGateway{err: errors.New("gateway unavailable")})

	_, err := service.StartPayment(context.Background(), checkoutDomain.StartPaymentRequest{Preparation: checkoutDomain.PrepareRequest{CheckoutID: uuid.New(), Locale: "es", ExpiresAt: time.Now().Add(time.Minute), Lines: []checkoutDomain.Line{{VariantID: uuid.New(), WarehouseID: uuid.New(), Quantity: 1}}}, OrderNumber: "ES-201"})
	if err == nil || workflow.cancelled == uuid.Nil {
		t.Fatalf("StartPayment() error = %v, workflow = %+v", err, workflow)
	}
}

func TestStartPaymentReleasesReservationWhenOrderWorkflowFails(t *testing.T) {
	inventory := &fakeInventory{}
	price, _ := money.New(1000, "EUR")
	workflow := &fakeWorkflow{err: errors.New("cannot persist order")}
	service := NewService(inventory, &fakeVariantFinder{price: price}, mustTaxPolicy(t, tax.ModeNone, 0), checkoutDomain.Policy{AllowGuest: true}, workflow, &fakeGateway{})

	_, err := service.StartPayment(context.Background(), checkoutDomain.StartPaymentRequest{Preparation: checkoutDomain.PrepareRequest{CheckoutID: uuid.New(), Locale: "es", ExpiresAt: time.Now().Add(time.Minute), Lines: []checkoutDomain.Line{{VariantID: uuid.New(), WarehouseID: uuid.New(), Quantity: 1}}}, OrderNumber: "ES-202"})
	if err == nil || len(inventory.released) != 1 {
		t.Fatalf("StartPayment() error = %v, released = %v", err, inventory.released)
	}
}

func TestStartPaymentAppliesCheckoutPolicyBeforeReservation(t *testing.T) {
	inventory := &fakeInventory{}
	price, _ := money.New(1000, "EUR")
	service := NewService(inventory, &fakeVariantFinder{price: price}, mustTaxPolicy(t, tax.ModeNone, 0), checkoutDomain.Policy{AllowGuest: false, RequirePhone: true}, &fakeWorkflow{}, &fakeGateway{})

	_, err := service.StartPayment(context.Background(), checkoutDomain.StartPaymentRequest{Preparation: checkoutDomain.PrepareRequest{CheckoutID: uuid.New(), Locale: "es", ExpiresAt: time.Now().Add(time.Minute), Lines: []checkoutDomain.Line{{VariantID: uuid.New(), WarehouseID: uuid.New(), Quantity: 1}}}, OrderNumber: "ES-203"})
	if err == nil || len(inventory.batch) != 0 {
		t.Fatalf("StartPayment() error = %v, reservations = %+v", err, inventory.batch)
	}
}

func TestStartPaymentRejectsDisabledDeliveryProviderBeforeReservation(t *testing.T) {
	inventory := &fakeInventory{}
	price, _ := money.New(1000, "EUR")
	service := NewService(inventory, &fakeVariantFinder{price: price}, mustTaxPolicy(t, tax.ModeNone, 0), checkoutDomain.Policy{AllowGuest: true, SupportedDeliveryProviders: []string{"correos"}, DefaultDeliveryProvider: "correos"}, &fakeWorkflow{}, &fakeGateway{})

	_, err := service.StartPayment(context.Background(), checkoutDomain.StartPaymentRequest{Preparation: checkoutDomain.PrepareRequest{CheckoutID: uuid.New(), Locale: "es", ExpiresAt: time.Now().Add(time.Minute), Lines: []checkoutDomain.Line{{VariantID: uuid.New(), WarehouseID: uuid.New(), Quantity: 1}}}, DeliveryProvider: "novaposhta"})
	if err == nil || len(inventory.batch) != 0 {
		t.Fatalf("StartPayment() error = %v, reservations = %+v", err, inventory.batch)
	}
}

func TestStartPaymentUsesConfiguredDefaultDeliveryProvider(t *testing.T) {
	inventory := &fakeInventory{}
	price, _ := money.New(1000, "EUR")
	workflow := &fakeWorkflow{}
	service := NewService(inventory, &fakeVariantFinder{price: price}, mustTaxPolicy(t, tax.ModeNone, 0), checkoutDomain.Policy{AllowGuest: true, SupportedDeliveryProviders: []string{"novaposhta"}, DefaultDeliveryProvider: "novaposhta"}, workflow, &fakeGateway{})

	_, err := service.StartPayment(context.Background(), checkoutDomain.StartPaymentRequest{Preparation: checkoutDomain.PrepareRequest{CheckoutID: uuid.New(), Locale: "es", ExpiresAt: time.Now().Add(time.Minute), Lines: []checkoutDomain.Line{{VariantID: uuid.New(), WarehouseID: uuid.New(), Quantity: 1}}}, Delivery: &checkoutDomain.DeliveryDetails{RecipientName: "Iryna Customer", RecipientPhone: "+34123456789"}})
	if err != nil {
		t.Fatalf("StartPayment() error = %v", err)
	}
	if workflow.created == nil || workflow.created.DeliveryProvider != "novaposhta" {
		t.Fatalf("created order = %+v", workflow.created)
	}
}

func TestStartPaymentRequiresRecipientDetailsForEnabledDelivery(t *testing.T) {
	inventory := &fakeInventory{}
	price, _ := money.New(1000, "EUR")
	service := NewService(inventory, &fakeVariantFinder{price: price}, mustTaxPolicy(t, tax.ModeNone, 0), checkoutDomain.Policy{AllowGuest: true, SupportedDeliveryProviders: []string{"novaposhta"}, DefaultDeliveryProvider: "novaposhta"}, &fakeWorkflow{}, &fakeGateway{})
	_, err := service.StartPayment(context.Background(), checkoutDomain.StartPaymentRequest{Preparation: checkoutDomain.PrepareRequest{CheckoutID: uuid.New(), Locale: "es", ExpiresAt: time.Now().Add(time.Minute), Lines: []checkoutDomain.Line{{VariantID: uuid.New(), WarehouseID: uuid.New(), Quantity: 1}}}})
	if err == nil || len(inventory.batch) != 0 {
		t.Fatalf("StartPayment() error = %v, reservations = %+v", err, inventory.batch)
	}
}

func TestStartPaymentGeneratesConfiguredOrderNumberWhenNotSupplied(t *testing.T) {
	inventory := &fakeInventory{}
	price, _ := money.New(1000, "EUR")
	workflow := &fakeWorkflow{}
	checkoutID := uuid.New()
	service := NewService(inventory, &fakeVariantFinder{price: price}, mustTaxPolicy(t, tax.ModeNone, 0), checkoutDomain.Policy{AllowGuest: true, OrderNumberPrefix: "cosmetics-es"}, workflow, &fakeGateway{})
	if _, err := service.StartPayment(context.Background(), checkoutDomain.StartPaymentRequest{Preparation: checkoutDomain.PrepareRequest{CheckoutID: checkoutID, Locale: "es", ExpiresAt: time.Now().Add(time.Minute), Lines: []checkoutDomain.Line{{VariantID: uuid.New(), WarehouseID: uuid.New(), Quantity: 1}}}}); err != nil {
		t.Fatalf("StartPayment() error = %v", err)
	}
	if workflow.created == nil || workflow.created.Number != "COSMETICS-ES-"+strings.ToUpper(checkoutID.String()[:8]) {
		t.Fatalf("generated order = %+v", workflow.created)
	}
}

type fakeVariantFinder struct {
	price money.Money
	err   error
}

func (f *fakeVariantFinder) FindActiveForCheckout(_ context.Context, variantID uuid.UUID, _ string) (*catalogDomain.CheckoutVariant, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &catalogDomain.CheckoutVariant{VariantID: variantID, ProductID: uuid.New(), ProductName: "Cream", SKU: "CREAM-50", UnitPrice: f.price}, nil
}

func mustTaxPolicy(t *testing.T, mode tax.Mode, rate int) tax.Calculator {
	t.Helper()
	policy, err := tax.NewPolicy(mode, rate)
	if err != nil {
		t.Fatalf("NewPolicy() error = %v", err)
	}
	return policy
}

type fakeInventory struct {
	batch    []inventoryDomain.ReservationRequest
	released []uuid.UUID
}

func (f *fakeInventory) Reserve(context.Context, inventoryDomain.ReservationRequest) (*inventoryDomain.Reservation, error) {
	return nil, nil
}
func (f *fakeInventory) ReserveBatch(_ context.Context, qs []inventoryDomain.ReservationRequest) ([]inventoryDomain.Reservation, error) {
	f.batch = qs
	result := make([]inventoryDomain.Reservation, 0, len(qs))
	for _, q := range qs {
		result = append(result, inventoryDomain.Reservation{ID: uuid.New(), IdempotencyKey: q.IdempotencyKey})
	}
	return result, nil
}
func (f *fakeInventory) Release(_ context.Context, reservationID uuid.UUID) error {
	f.released = append(f.released, reservationID)
	return nil
}
func (f *fakeInventory) Commit(context.Context, uuid.UUID, uuid.UUID) error      { return nil }
func (f *fakeInventory) Adjust(context.Context, uuid.UUID, uuid.UUID, int) error { return nil }

type fakeWorkflow struct {
	created   *ordersDomain.Order
	cancelled uuid.UUID
	paid      uuid.UUID
	err       error
}

func (f *fakeWorkflow) CreatePending(_ context.Context, draft ordersDomain.Draft, _ []uuid.UUID) (*ordersDomain.Order, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.created = &ordersDomain.Order{ID: uuid.New(), Number: draft.Number, Total: draft.Total, DeliveryProvider: draft.DeliveryProvider}
	return f.created, nil
}
func (f *fakeWorkflow) CancelPending(_ context.Context, orderID uuid.UUID) error {
	f.cancelled = orderID
	return nil
}
func (f *fakeWorkflow) MarkPaid(_ context.Context, orderID uuid.UUID) error {
	f.paid = orderID
	return nil
}

type fakeGateway struct {
	payment paymentsDomain.CheckoutPayment
	err     error
}

func (f *fakeGateway) Code() string { return "fake" }
func (f *fakeGateway) CreateCheckout(_ context.Context, payment paymentsDomain.CheckoutPayment) (paymentsDomain.PaymentSession, error) {
	f.payment = payment
	if f.err != nil {
		return paymentsDomain.PaymentSession{}, f.err
	}
	return paymentsDomain.PaymentSession{ProviderReference: "fake-payment"}, nil
}
func (f *fakeGateway) VerifyWebhook(context.Context, paymentsDomain.WebhookRequest) (paymentsDomain.PaymentEvent, error) {
	return paymentsDomain.PaymentEvent{}, nil
}
func (f *fakeGateway) Refund(context.Context, paymentsDomain.RefundRequest) error { return nil }

var _ workflowDomain.Service = (*fakeWorkflow)(nil)
var _ paymentsDomain.Gateway = (*fakeGateway)(nil)
