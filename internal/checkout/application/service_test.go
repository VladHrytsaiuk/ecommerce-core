package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	catalogDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/domain"
	checkoutDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/checkout/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
	workflowDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/core/orderworkflow/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/tax"
	deliveryDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/delivery/domain"
	inventoryDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/inventory/domain"
	ordersDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/orders/domain"
	paymentsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/payments/domain"
	"github.com/google/uuid"
)

func TestPreparePaymentAggregatesDuplicateLinesIntoOneAtomicBatch(t *testing.T) {
	inventory := &fakeInventory{}
	price := mustMoney(1000, "EUR")
	service := NewService(inventory, &fakeVariantFinder{price: price}, mustTaxPolicy(t, tax.ModeVATExcluded, 20), checkoutDomain.Policy{AllowGuest: true}, nil, nil)
	variant, warehouse := uuid.New(), uuid.New()
	prepared, err := service.PreparePayment(context.Background(), checkoutDomain.PrepareRequest{CheckoutID: uuid.New(), Locale: "es", ExpiresAt: time.Now().Add(time.Minute), Lines: []checkoutDomain.Line{{VariantID: variant, WarehouseID: warehouse, Quantity: 1}, {VariantID: variant, WarehouseID: warehouse, Quantity: 2}}})
	if err != nil {
		t.Fatalf("PreparePayment() error = %v", err)
	}
	if len(inventory.batch) != 1 || inventory.batch[0].Quantity != 3 || len(prepared.ReservationIDs) != 1 || len(prepared.Items) != 1 || prepared.Items[0].UnitWeightGrams != 250 || prepared.Subtotal.Amount() != 3000 || prepared.Tax.Amount() != 600 || prepared.Total.Amount() != 3600 {
		t.Fatalf("batch = %+v, prepared = %+v", inventory.batch, prepared)
	}
}

func TestPreparePaymentDoesNotReserveWhenCatalogVariantIsUnavailable(t *testing.T) {
	inventory := &fakeInventory{}
	price := mustMoney(1000, "EUR")
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
	price := mustMoney(1000, "EUR")
	workflow := &fakeWorkflow{}
	gateway := &fakeGateway{}
	service := NewService(inventory, &fakeVariantFinder{price: price}, mustTaxPolicy(t, tax.ModeNone, 0), checkoutDomain.Policy{AllowGuest: true}, workflow, gateway)
	checkoutID := uuid.New()

	started, err := service.StartPayment(context.Background(), checkoutDomain.StartPaymentRequest{Preparation: checkoutDomain.PrepareRequest{CheckoutID: checkoutID, Locale: "es", ExpiresAt: time.Now().Add(time.Minute), Lines: []checkoutDomain.Line{{VariantID: uuid.New(), WarehouseID: uuid.New(), Quantity: 1}}}, CustomerEmail: "buyer@example.com", OrderNumber: "ES-200", ReturnURL: "https://store.example/return"})
	if err != nil {
		t.Fatalf("StartPayment() error = %v", err)
	}
	if workflow.created == nil || gateway.payment.OrderID != workflow.created.ID || gateway.payment.IdempotencyKey != checkoutID.String() || started.Session.ProviderReference == "" {
		t.Fatalf("workflow/gateway state = %+v / %+v", workflow, gateway)
	}
}

func TestStartPaymentCompletesZeroTotalCheckoutWithoutGateway(t *testing.T) {
	inventory := &fakeInventory{}
	price := mustMoney(1000, "EUR")
	workflow := &fakeWorkflow{}
	prices := &zeroPriceCalculator{}
	service := NewService(inventory, &fakeVariantFinder{price: price}, mustTaxPolicy(t, tax.ModeNone, 0), checkoutDomain.Policy{AllowGuest: true}, workflow, nil).WithPriceCalculator(prices)
	started, err := service.StartPayment(context.Background(), checkoutDomain.StartPaymentRequest{Preparation: checkoutDomain.PrepareRequest{CheckoutID: uuid.New(), Locale: "es", ExpiresAt: time.Now().Add(time.Minute), Lines: []checkoutDomain.Line{{VariantID: uuid.New(), WarehouseID: uuid.New(), Quantity: 1}}}, CustomerEmail: "buyer@example.com", OrderNumber: "FREE-1"})
	if err != nil {
		t.Fatalf("StartPayment() error = %v", err)
	}
	if started.Order.Status != ordersDomain.StatusPaid || started.Order.PaymentProvider != "free" || started.Session.ProviderReference != "" {
		t.Fatalf("zero-total checkout = %+v", started)
	}
}

func TestStartPaymentCancelsOrderWhenGatewayFails(t *testing.T) {
	inventory := &fakeInventory{}
	price := mustMoney(1000, "EUR")
	workflow := &fakeWorkflow{}
	service := NewService(inventory, &fakeVariantFinder{price: price}, mustTaxPolicy(t, tax.ModeNone, 0), checkoutDomain.Policy{AllowGuest: true}, workflow, &fakeGateway{err: fmt.Errorf("%w: gateway unavailable", paymentsDomain.ErrGatewayRejected)})

	_, err := service.StartPayment(context.Background(), checkoutDomain.StartPaymentRequest{Preparation: checkoutDomain.PrepareRequest{CheckoutID: uuid.New(), Locale: "es", ExpiresAt: time.Now().Add(time.Minute), Lines: []checkoutDomain.Line{{VariantID: uuid.New(), WarehouseID: uuid.New(), Quantity: 1}}}, CustomerEmail: "buyer@example.com", OrderNumber: "ES-201"})
	if err == nil || workflow.cancelled == uuid.Nil {
		t.Fatalf("StartPayment() error = %v, workflow = %+v", err, workflow)
	}
}

func TestStartPaymentReplaysExistingCheckoutWithoutNewOrderOrReservation(t *testing.T) {
	amount := mustMoney(1000, "EUR")
	attempt := &workflowDomain.CheckoutAttempt{OrderID: uuid.New(), OrderNumber: "ES-REPLAY", OrderStatus: ordersDomain.StatusPendingPayment, Provider: "fake", IdempotencyKey: "stable-checkout-key", Amount: amount, Status: "created"}
	workflow := &fakeWorkflow{checkoutAttempt: attempt}
	inventory := &fakeInventory{}
	gateway := &fakeGateway{}
	service := NewService(inventory, &fakeVariantFinder{price: amount}, mustTaxPolicy(t, tax.ModeNone, 0), checkoutDomain.Policy{AllowGuest: true}, workflow, gateway)
	checkoutID := uuid.New()
	attempt.IdempotencyKey = checkoutID.String()

	started, err := service.StartPayment(context.Background(), checkoutDomain.StartPaymentRequest{Preparation: checkoutDomain.PrepareRequest{CheckoutID: checkoutID, Locale: "es", ExpiresAt: time.Now().Add(time.Minute), Lines: []checkoutDomain.Line{{VariantID: uuid.New(), WarehouseID: uuid.New(), Quantity: 1}}}, CustomerEmail: "buyer@example.com", ReturnURL: "https://store.example/return"})
	if err != nil {
		t.Fatalf("StartPayment() error = %v", err)
	}
	if started.Order.ID != attempt.OrderID || gateway.payment.OrderID != attempt.OrderID || gateway.payment.IdempotencyKey != attempt.IdempotencyKey || workflow.created != nil || len(inventory.batch) != 0 {
		t.Fatalf("started=%+v gateway=%+v workflow=%+v reservations=%+v", started, gateway.payment, workflow, inventory.batch)
	}
}

func TestStartPaymentReleasesReservationWhenOrderWorkflowFails(t *testing.T) {
	inventory := &fakeInventory{}
	price := mustMoney(1000, "EUR")
	workflow := &fakeWorkflow{err: errors.New("cannot persist order")}
	service := NewService(inventory, &fakeVariantFinder{price: price}, mustTaxPolicy(t, tax.ModeNone, 0), checkoutDomain.Policy{AllowGuest: true}, workflow, &fakeGateway{})

	_, err := service.StartPayment(context.Background(), checkoutDomain.StartPaymentRequest{Preparation: checkoutDomain.PrepareRequest{CheckoutID: uuid.New(), Locale: "es", ExpiresAt: time.Now().Add(time.Minute), Lines: []checkoutDomain.Line{{VariantID: uuid.New(), WarehouseID: uuid.New(), Quantity: 1}}}, CustomerEmail: "buyer@example.com", OrderNumber: "ES-202"})
	if err == nil || len(inventory.released) != 1 {
		t.Fatalf("StartPayment() error = %v, released = %v", err, inventory.released)
	}
}

func TestStartPaymentAppliesCheckoutPolicyBeforeReservation(t *testing.T) {
	inventory := &fakeInventory{}
	price := mustMoney(1000, "EUR")
	service := NewService(inventory, &fakeVariantFinder{price: price}, mustTaxPolicy(t, tax.ModeNone, 0), checkoutDomain.Policy{AllowGuest: false, RequirePhone: true}, &fakeWorkflow{}, &fakeGateway{})

	_, err := service.StartPayment(context.Background(), checkoutDomain.StartPaymentRequest{Preparation: checkoutDomain.PrepareRequest{CheckoutID: uuid.New(), Locale: "es", ExpiresAt: time.Now().Add(time.Minute), Lines: []checkoutDomain.Line{{VariantID: uuid.New(), WarehouseID: uuid.New(), Quantity: 1}}}, CustomerEmail: "buyer@example.com", OrderNumber: "ES-203"})
	if err == nil || len(inventory.batch) != 0 {
		t.Fatalf("StartPayment() error = %v, reservations = %+v", err, inventory.batch)
	}
}

func TestStartPaymentRequiresCustomerEmailBeforeReservation(t *testing.T) {
	inventory := &fakeInventory{}
	price := mustMoney(1000, "EUR")
	service := NewService(inventory, &fakeVariantFinder{price: price}, mustTaxPolicy(t, tax.ModeNone, 0), checkoutDomain.Policy{AllowGuest: true}, &fakeWorkflow{}, &fakeGateway{})

	_, err := service.StartPayment(context.Background(), checkoutDomain.StartPaymentRequest{Preparation: checkoutDomain.PrepareRequest{CheckoutID: uuid.New(), Locale: "es", ExpiresAt: time.Now().Add(time.Minute), Lines: []checkoutDomain.Line{{VariantID: uuid.New(), WarehouseID: uuid.New(), Quantity: 1}}}})
	if err == nil || len(inventory.batch) != 0 {
		t.Fatalf("StartPayment() error = %v, reservations = %+v", err, inventory.batch)
	}
}

func TestStartPaymentRejectsDisabledDeliveryProviderBeforeReservation(t *testing.T) {
	inventory := &fakeInventory{}
	price := mustMoney(1000, "EUR")
	service := NewService(inventory, &fakeVariantFinder{price: price}, mustTaxPolicy(t, tax.ModeNone, 0), checkoutDomain.Policy{AllowGuest: true, SupportedDeliveryProviders: []string{"correos"}, DefaultDeliveryProvider: "correos"}, &fakeWorkflow{}, &fakeGateway{})

	_, err := service.StartPayment(context.Background(), checkoutDomain.StartPaymentRequest{Preparation: checkoutDomain.PrepareRequest{CheckoutID: uuid.New(), Locale: "es", ExpiresAt: time.Now().Add(time.Minute), Lines: []checkoutDomain.Line{{VariantID: uuid.New(), WarehouseID: uuid.New(), Quantity: 1}}}, CustomerEmail: "buyer@example.com", DeliveryProvider: "novaposhta"})
	if err == nil || len(inventory.batch) != 0 {
		t.Fatalf("StartPayment() error = %v, reservations = %+v", err, inventory.batch)
	}
}

func TestStartPaymentUsesConfiguredDefaultDeliveryProvider(t *testing.T) {
	inventory := &fakeInventory{}
	price := mustMoney(1000, "EUR")
	workflow := &fakeWorkflow{}
	service := NewService(inventory, &fakeVariantFinder{price: price}, mustTaxPolicy(t, tax.ModeNone, 0), checkoutDomain.Policy{AllowGuest: true, SupportedDeliveryProviders: []string{"novaposhta"}, DefaultDeliveryProvider: "novaposhta"}, workflow, &fakeGateway{}).WithCarriers(fakeCarriers{"novaposhta": &fakeCarrier{}})

	_, err := service.StartPayment(context.Background(), checkoutDomain.StartPaymentRequest{Preparation: checkoutDomain.PrepareRequest{CheckoutID: uuid.New(), Locale: "es", ExpiresAt: time.Now().Add(time.Minute), Lines: []checkoutDomain.Line{{VariantID: uuid.New(), WarehouseID: uuid.New(), Quantity: 1}}}, CustomerEmail: "buyer@example.com", DeliveryOptionCode: "standard", Delivery: &checkoutDomain.DeliveryDetails{RecipientName: "Iryna Customer", RecipientPhone: "+34123456789"}})
	if err != nil {
		t.Fatalf("StartPayment() error = %v", err)
	}
	if workflow.created == nil || workflow.created.DeliveryProvider != "novaposhta" {
		t.Fatalf("created order = %+v", workflow.created)
	}
	if workflow.created.Shipping.Amount() != 500 || workflow.created.Total.Amount() != 1500 {
		t.Fatalf("delivery total snapshot = %+v", workflow.created)
	}
}

func TestStartPaymentReleasesReservationWhenSelectedDeliveryOptionIsUnavailable(t *testing.T) {
	inventory := &fakeInventory{}
	price := mustMoney(1000, "EUR")
	service := NewService(inventory, &fakeVariantFinder{price: price}, mustTaxPolicy(t, tax.ModeNone, 0), checkoutDomain.Policy{AllowGuest: true, SupportedDeliveryProviders: []string{"carrier"}, DefaultDeliveryProvider: "carrier"}, &fakeWorkflow{}, &fakeGateway{}).WithCarriers(fakeCarriers{"carrier": &fakeCarrier{}})

	_, err := service.StartPayment(context.Background(), checkoutDomain.StartPaymentRequest{Preparation: checkoutDomain.PrepareRequest{CheckoutID: uuid.New(), Locale: "es", ExpiresAt: time.Now().Add(time.Minute), Lines: []checkoutDomain.Line{{VariantID: uuid.New(), WarehouseID: uuid.New(), Quantity: 1}}}, CustomerEmail: "buyer@example.com", DeliveryOptionCode: "express", Delivery: &checkoutDomain.DeliveryDetails{RecipientName: "Iryna Customer", RecipientPhone: "+34123456789"}})
	if err == nil || len(inventory.released) != 1 {
		t.Fatalf("StartPayment() error=%v released=%v", err, inventory.released)
	}
}

func TestStartPaymentRequiresRecipientDetailsForEnabledDelivery(t *testing.T) {
	inventory := &fakeInventory{}
	price := mustMoney(1000, "EUR")
	service := NewService(inventory, &fakeVariantFinder{price: price}, mustTaxPolicy(t, tax.ModeNone, 0), checkoutDomain.Policy{AllowGuest: true, SupportedDeliveryProviders: []string{"novaposhta"}, DefaultDeliveryProvider: "novaposhta"}, &fakeWorkflow{}, &fakeGateway{})
	_, err := service.StartPayment(context.Background(), checkoutDomain.StartPaymentRequest{Preparation: checkoutDomain.PrepareRequest{CheckoutID: uuid.New(), Locale: "es", ExpiresAt: time.Now().Add(time.Minute), Lines: []checkoutDomain.Line{{VariantID: uuid.New(), WarehouseID: uuid.New(), Quantity: 1}}}, CustomerEmail: "buyer@example.com"})
	if err == nil || len(inventory.batch) != 0 {
		t.Fatalf("StartPayment() error = %v, reservations = %+v", err, inventory.batch)
	}
}

func TestStartPaymentGeneratesConfiguredOrderNumberWhenNotSupplied(t *testing.T) {
	inventory := &fakeInventory{}
	price := mustMoney(1000, "EUR")
	workflow := &fakeWorkflow{}
	checkoutID := uuid.New()
	service := NewService(inventory, &fakeVariantFinder{price: price}, mustTaxPolicy(t, tax.ModeNone, 0), checkoutDomain.Policy{AllowGuest: true, OrderNumberPrefix: "cosmetics-es"}, workflow, &fakeGateway{})
	if _, err := service.StartPayment(context.Background(), checkoutDomain.StartPaymentRequest{Preparation: checkoutDomain.PrepareRequest{CheckoutID: checkoutID, Locale: "es", ExpiresAt: time.Now().Add(time.Minute), Lines: []checkoutDomain.Line{{VariantID: uuid.New(), WarehouseID: uuid.New(), Quantity: 1}}}, CustomerEmail: "buyer@example.com"}); err != nil {
		t.Fatalf("StartPayment() error = %v", err)
	}
	if workflow.created == nil || workflow.created.Number != "COSMETICS-ES-"+strings.ToUpper(checkoutID.String()[:8]) {
		t.Fatalf("generated order = %+v", workflow.created)
	}
}

func TestQuoteDeliveryUsesCatalogWeightAndEnabledCarrier(t *testing.T) {
	price := mustMoney(1000, "EUR")
	variant := uuid.New()
	carrier := &fakeCarrier{}
	service := NewService(&fakeInventory{}, &fakeVariantFinder{price: price}, mustTaxPolicy(t, tax.ModeNone, 0), checkoutDomain.Policy{AllowGuest: true, SupportedDeliveryProviders: []string{"carrier"}, DefaultDeliveryProvider: "carrier"}, nil, nil).WithCarriers(fakeCarriers{"carrier": carrier})

	quote, err := service.QuoteDelivery(context.Background(), checkoutDomain.DeliveryQuoteRequest{Locale: "es", Lines: []checkoutDomain.Line{{VariantID: variant, Quantity: 2}}, Delivery: checkoutDomain.DeliveryDetails{City: "Madrid"}})
	if err != nil {
		t.Fatal(err)
	}
	if quote.Provider != "carrier" || len(carrier.request.Items) != 1 || carrier.request.Items[0].VariantID != variant || carrier.request.Items[0].Quantity != 2 || carrier.request.Items[0].WeightGrams != 250 || carrier.request.Currency != "EUR" {
		t.Fatalf("quote=%+v carrier request=%+v", quote, carrier.request)
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
	return &catalogDomain.CheckoutVariant{VariantID: variantID, ProductID: uuid.New(), ProductName: "Cream", SKU: "CREAM-50", UnitPrice: f.price, WeightGrams: 250}, nil
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
	created         *ordersDomain.Order
	checkoutAttempt *workflowDomain.CheckoutAttempt
	cancelled       uuid.UUID
	paid            workflowDomain.PaymentConfirmation
	registered      workflowDomain.PaymentAttempt
	err             error
}

func (f *fakeWorkflow) RegisterPayment(_ context.Context, attempt workflowDomain.PaymentAttempt) error {
	f.registered = attempt
	return f.err
}
func (*fakeWorkflow) RecordCheckoutAttempt(context.Context, workflowDomain.CheckoutAttemptRequest) error {
	return nil
}
func (f *fakeWorkflow) FindCheckoutAttempt(_ context.Context, key string) (*workflowDomain.CheckoutAttempt, error) {
	if f.checkoutAttempt != nil && f.checkoutAttempt.IdempotencyKey == key {
		return f.checkoutAttempt, nil
	}
	return nil, nil
}
func (*fakeWorkflow) ClaimPendingCheckoutAttempt(context.Context, time.Duration, time.Duration) (*workflowDomain.CheckoutAttempt, error) {
	return nil, nil
}
func (*fakeWorkflow) MarkCheckoutAttemptFailed(context.Context, uuid.UUID) error   { return nil }
func (*fakeWorkflow) RetryCheckoutAttempt(context.Context, uuid.UUID, error) error { return nil }

func (f *fakeWorkflow) CreatePending(_ context.Context, draft ordersDomain.Draft, _ []uuid.UUID) (*ordersDomain.Order, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.created = &ordersDomain.Order{ID: uuid.New(), Number: draft.Number, Total: draft.Total, Shipping: draft.Shipping, DeliveryProvider: draft.DeliveryProvider}
	return f.created, nil
}
func (f *fakeWorkflow) CreatePendingCheckout(ctx context.Context, draft ordersDomain.Draft, reservations []uuid.UUID, _ workflowDomain.CheckoutAttemptRequest) (*ordersDomain.Order, error) {
	return f.CreatePending(ctx, draft, reservations)
}
func (f *fakeWorkflow) CreatePaidCheckout(ctx context.Context, draft ordersDomain.Draft, reservations []uuid.UUID, _ workflowDomain.CheckoutAttemptRequest) (*ordersDomain.Order, error) {
	order, err := f.CreatePending(ctx, draft, reservations)
	if order != nil {
		order.Status = ordersDomain.StatusPaid
		order.PaymentProvider = "free"
	}
	return order, err
}
func (*fakeWorkflow) ExpirePendingCheckout(context.Context, time.Time) (bool, error) {
	return false, nil
}
func (f *fakeWorkflow) CancelPending(_ context.Context, orderID uuid.UUID) error {
	f.cancelled = orderID
	return nil
}
func (f *fakeWorkflow) MarkPaid(_ context.Context, confirmation workflowDomain.PaymentConfirmation) error {
	f.paid = confirmation
	return nil
}
func (*fakeWorkflow) MarkFailed(context.Context, workflowDomain.PaymentConfirmation) error {
	return nil
}
func (*fakeWorkflow) MarkRefunded(context.Context, workflowDomain.PaymentConfirmation) error {
	return nil
}

type fakeGateway struct {
	payment paymentsDomain.CheckoutPayment
	err     error
}

type fakeCarriers map[string]deliveryDomain.Carrier

func (f fakeCarriers) Get(code string) (deliveryDomain.Carrier, bool) {
	carrier, ok := f[code]
	return carrier, ok
}

type fakeCarrier struct {
	request deliveryDomain.ShipmentQuoteRequest
}

func (*fakeCarrier) Code() string { return "carrier" }
func (f *fakeCarrier) Quote(_ context.Context, request deliveryDomain.ShipmentQuoteRequest) ([]deliveryDomain.ShippingOption, error) {
	f.request = request
	amount := mustMoney(500, request.Currency)
	return []deliveryDomain.ShippingOption{{Code: "standard", Amount: amount}}, nil
}
func (*fakeCarrier) CreateShipment(context.Context, deliveryDomain.CreateShipmentRequest) (deliveryDomain.ShipmentResult, error) {
	return deliveryDomain.ShipmentResult{}, nil
}
func (*fakeCarrier) Track(context.Context, deliveryDomain.TrackingRequest) (deliveryDomain.TrackingResult, error) {
	return deliveryDomain.TrackingResult{}, nil
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

type zeroPriceCalculator struct{}

func (zeroPriceCalculator) Calculate(_ context.Context, request checkoutDomain.PriceCalculationRequest) (checkoutDomain.Price, error) {
	zero := mustMoney(0, request.Subtotal.Currency())
	return checkoutDomain.Price{Subtotal: zero, Discount: request.Subtotal, Tax: zero, Total: zero}, nil
}

var _ paymentsDomain.Gateway = (*fakeGateway)(nil)
