package application

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
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

type variantFinder interface {
	FindActiveForCheckout(context.Context, uuid.UUID, string) (*catalogDomain.CheckoutVariant, error)
}

type carrierFinder interface {
	Get(string) (deliveryDomain.Carrier, bool)
}

type Service struct {
	inventory inventoryDomain.Service
	variants  variantFinder
	tax       tax.Calculator
	policy    checkoutDomain.Policy
	workflow  workflowDomain.Service
	gateway   paymentsDomain.Gateway
	carriers  carrierFinder
}

func NewService(inventory inventoryDomain.Service, variants variantFinder, tax tax.Calculator, policy checkoutDomain.Policy, workflow workflowDomain.Service, gateway paymentsDomain.Gateway) *Service {
	return &Service{inventory: inventory, variants: variants, tax: tax, policy: policy, workflow: workflow, gateway: gateway}
}

// WithCarriers attaches the configured delivery registry to checkout without
// making the domain depend on an adapter or the delivery application package.
func (s *Service) WithCarriers(carriers carrierFinder) *Service {
	s.carriers = carriers
	return s
}

// QuoteDelivery translates immutable server-side cart lines into the neutral
// Carrier contract. It deliberately performs no reservation or persistence;
// carrier I/O therefore remains outside all database transactions.
func (s *Service) QuoteDelivery(ctx context.Context, request checkoutDomain.DeliveryQuoteRequest) (*checkoutDomain.DeliveryQuote, error) {
	provider, err := s.policy.ResolveDeliveryProvider(request.DeliveryProvider)
	if err != nil {
		return nil, err
	}
	if provider == "" || s.carriers == nil {
		return nil, fmt.Errorf("delivery quotes are not configured")
	}
	if strings.TrimSpace(request.Locale) == "" || len(request.Lines) == 0 {
		return nil, fmt.Errorf("invalid delivery quote request")
	}
	carrier, ok := s.carriers.Get(provider)
	if !ok {
		return nil, fmt.Errorf("delivery carrier %q is not enabled", provider)
	}
	quantities := make(map[uuid.UUID]int, len(request.Lines))
	for _, line := range request.Lines {
		if line.VariantID == uuid.Nil || line.Quantity <= 0 {
			return nil, fmt.Errorf("invalid delivery quote line")
		}
		quantities[line.VariantID] += line.Quantity
	}
	items, _, err := s.snapshotItems(ctx, quantities, request.Locale)
	if err != nil {
		return nil, err
	}
	shipmentItems := make([]deliveryDomain.ShipmentItem, 0, len(items))
	for _, item := range items {
		if item.VariantID == nil {
			return nil, fmt.Errorf("delivery quote item has no variant")
		}
		shipmentItems = append(shipmentItems, deliveryDomain.ShipmentItem{VariantID: *item.VariantID, Quantity: item.Quantity, WeightGrams: item.UnitWeightGrams})
	}
	options, err := carrier.Quote(ctx, deliveryDomain.ShipmentQuoteRequest{Destination: deliveryDomain.Address{RecipientName: request.Delivery.RecipientName, RecipientPhone: request.Delivery.RecipientPhone, CountryCode: request.Delivery.CountryCode, PostalCode: request.Delivery.PostalCode, City: request.Delivery.City, Line1: request.Delivery.Line1, Line2: request.Delivery.Line2, LocalityID: request.Delivery.LocalityID, ServicePointID: request.Delivery.ServicePointID}, Items: shipmentItems, Currency: items[0].Total.Currency})
	if err != nil {
		return nil, fmt.Errorf("quote delivery: %w", err)
	}
	return &checkoutDomain.DeliveryQuote{Provider: provider, Options: options}, nil
}

func (s *Service) PreparePayment(ctx context.Context, request checkoutDomain.PrepareRequest) (*checkoutDomain.PreparedCheckout, error) {
	if request.CheckoutID == uuid.Nil || strings.TrimSpace(request.Locale) == "" || len(request.Lines) == 0 || !request.ExpiresAt.After(time.Now()) {
		return nil, fmt.Errorf("invalid checkout preparation")
	}
	type key struct{ variant, warehouse uuid.UUID }
	aggregated := map[key]int{}
	itemQuantities := map[uuid.UUID]int{}
	for _, line := range request.Lines {
		if line.VariantID == uuid.Nil || line.WarehouseID == uuid.Nil || line.Quantity <= 0 {
			return nil, fmt.Errorf("invalid checkout line")
		}
		aggregated[key{line.VariantID, line.WarehouseID}] += line.Quantity
		itemQuantities[line.VariantID] += line.Quantity
	}
	items, subtotal, err := s.snapshotItems(ctx, itemQuantities, request.Locale)
	if err != nil {
		return nil, err
	}
	breakdown, err := s.tax.Calculate(subtotal)
	if err != nil {
		return nil, err
	}
	keys := make([]key, 0, len(aggregated))
	for k := range aggregated {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i].variant.String()+keys[i].warehouse.String() < keys[j].variant.String()+keys[j].warehouse.String()
	})
	requests := make([]inventoryDomain.ReservationRequest, 0, len(keys))
	for _, k := range keys {
		requests = append(requests, inventoryDomain.ReservationRequest{IdempotencyKey: uuid.NewSHA1(request.CheckoutID, []byte(k.variant.String()+":"+k.warehouse.String())), VariantID: k.variant, WarehouseID: k.warehouse, Quantity: aggregated[k], ExpiresAt: request.ExpiresAt})
	}
	reservations, err := s.inventory.ReserveBatch(ctx, requests)
	if err != nil {
		return nil, err
	}
	result := &checkoutDomain.PreparedCheckout{CheckoutID: request.CheckoutID, ExpiresAt: request.ExpiresAt, ReservationIDs: make([]uuid.UUID, 0, len(reservations)), Items: items, Subtotal: breakdown.Subtotal, Tax: breakdown.Tax, Total: breakdown.Total}
	for _, reservation := range reservations {
		result.ReservationIDs = append(result.ReservationIDs, reservation.ID)
	}
	return result, nil
}

// StartPayment creates the order and associates reservations before it performs
// provider I/O. A gateway failure is compensated through the atomic workflow.
func (s *Service) StartPayment(ctx context.Context, request checkoutDomain.StartPaymentRequest) (*checkoutDomain.StartedCheckout, error) {
	if s.gateway == nil || s.workflow == nil {
		return nil, fmt.Errorf("checkout payment workflow is not configured")
	}
	if strings.TrimSpace(s.gateway.Code()) == "" {
		return nil, fmt.Errorf("checkout payment gateway code is required")
	}
	if started, found, err := s.replayCheckout(ctx, request); err != nil || found {
		return started, err
	}
	if err := s.policy.ValidateCustomer(request.CustomerID, request.CustomerPhone); err != nil {
		return nil, err
	}
	provider, err := s.policy.ResolveDeliveryProvider(request.DeliveryProvider)
	if err != nil {
		return nil, err
	}
	request.DeliveryProvider = provider
	if provider != "" {
		if request.Delivery == nil || strings.TrimSpace(request.Delivery.RecipientName) == "" || strings.TrimSpace(request.Delivery.RecipientPhone) == "" {
			return nil, fmt.Errorf("delivery recipient details are required")
		}
	}
	if strings.TrimSpace(request.OrderNumber) == "" {
		request.OrderNumber = s.policy.OrderNumber(request.Preparation.CheckoutID)
	}
	prepared, err := s.PreparePayment(ctx, request.Preparation)
	if err != nil {
		return nil, err
	}
	shipping, err := s.resolveShipping(ctx, provider, request.DeliveryOptionCode, request.Delivery, prepared.Items)
	if err != nil {
		s.releasePrepared(ctx, prepared.ReservationIDs)
		return nil, err
	}
	prepared.Shipping = shipping
	prepared.Total, err = prepared.Total.Add(shipping)
	if err != nil {
		s.releasePrepared(ctx, prepared.ReservationIDs)
		return nil, fmt.Errorf("add shipping total: %w", err)
	}
	order, err := s.workflow.CreatePendingCheckout(ctx, ordersDomain.Draft{
		Number:           request.OrderNumber,
		CartID:           request.CartID,
		CustomerID:       request.CustomerID,
		Subtotal:         prepared.Subtotal,
		Tax:              prepared.Tax,
		Shipping:         prepared.Shipping,
		Total:            prepared.Total,
		PaymentProvider:  strings.TrimSpace(s.gateway.Code()),
		DeliveryProvider: strings.TrimSpace(request.DeliveryProvider),
		Delivery:         mapDelivery(request.Delivery),
		Items:            prepared.Items,
	}, prepared.ReservationIDs, workflowDomain.CheckoutAttemptRequest{
		Provider: s.gateway.Code(), IdempotencyKey: request.Preparation.CheckoutID.String(), Amount: prepared.Total,
	})
	if err != nil {
		s.releasePrepared(ctx, prepared.ReservationIDs)
		return nil, err
	}

	session, err := s.gateway.CreateCheckout(ctx, paymentsDomain.CheckoutPayment{
		OrderID:        order.ID,
		IdempotencyKey: request.Preparation.CheckoutID.String(),
		Amount:         prepared.Total,
		ReturnURL:      request.ReturnURL,
		CancelURL:      request.CancelURL,
	})
	if err != nil {
		// If gateway explicitly rejects, we fail the attempt and cancel the order.
		// If it's a network error or undetermined timeout, we leave it in "creating" state for recovery worker.
		if errors.Is(err, paymentsDomain.ErrGatewayRejected) {
			if markErr := s.workflow.MarkCheckoutAttemptFailed(context.WithoutCancel(ctx), order.ID); markErr != nil {
				return nil, fmt.Errorf("mark payment checkout rejected: %w", markErr)
			}
			if cancelErr := s.workflow.CancelPending(context.WithoutCancel(ctx), order.ID); cancelErr != nil {
				return nil, fmt.Errorf("create payment checkout rejected: %w; cancel pending order: %v", err, cancelErr)
			}
			return nil, fmt.Errorf("create payment checkout rejected: %w", err)
		}
		// Timeout or undetermined error: DO NOT cancel pending order.
		return nil, fmt.Errorf("create payment checkout network error (left pending): %w", err)
	}

	if err := s.workflow.RegisterPayment(ctx, workflowDomain.PaymentAttempt{OrderID: order.ID, Provider: s.gateway.Code(), ProviderReference: session.ProviderReference, Amount: prepared.Total}); err != nil {
		return nil, fmt.Errorf("record payment checkout (left pending for recovery): %w", err)
	}
	return &checkoutDomain.StartedCheckout{Prepared: prepared, Order: order, Session: session}, nil
}

// replayCheckout returns a new browser session for an existing logical
// checkout. Session secrets are never persisted: the enabled gateway receives
// the same provider idempotency key and supplies a fresh replay response.
func (s *Service) replayCheckout(ctx context.Context, request checkoutDomain.StartPaymentRequest) (*checkoutDomain.StartedCheckout, bool, error) {
	attempt, err := s.workflow.FindCheckoutAttempt(ctx, request.Preparation.CheckoutID.String())
	if err != nil {
		return nil, false, fmt.Errorf("find checkout attempt: %w", err)
	}
	if attempt == nil {
		return nil, false, nil
	}
	if attempt.Provider != s.gateway.Code() || attempt.OrderStatus != ordersDomain.StatusPendingPayment || attempt.Status == "failed" {
		return nil, true, fmt.Errorf("checkout attempt cannot be replayed")
	}
	session, err := s.gateway.CreateCheckout(ctx, paymentsDomain.CheckoutPayment{OrderID: attempt.OrderID, IdempotencyKey: attempt.IdempotencyKey, Amount: attempt.Amount, ReturnURL: request.ReturnURL, CancelURL: request.CancelURL})
	if err != nil {
		return nil, true, fmt.Errorf("replay payment checkout: %w", err)
	}
	if err := s.workflow.RegisterPayment(ctx, workflowDomain.PaymentAttempt{OrderID: attempt.OrderID, Provider: attempt.Provider, ProviderReference: session.ProviderReference, Amount: attempt.Amount}); err != nil {
		return nil, true, fmt.Errorf("register replayed payment checkout: %w", err)
	}
	return &checkoutDomain.StartedCheckout{Prepared: &checkoutDomain.PreparedCheckout{CheckoutID: request.Preparation.CheckoutID, ExpiresAt: request.Preparation.ExpiresAt, Total: attempt.Amount}, Order: &ordersDomain.Order{ID: attempt.OrderID, Number: attempt.OrderNumber, Status: attempt.OrderStatus, Total: attempt.Amount, PaymentProvider: attempt.Provider}, Session: session}, true, nil
}

func (s *Service) resolveShipping(ctx context.Context, provider, optionCode string, details *checkoutDomain.DeliveryDetails, items []ordersDomain.Item) (money.Money, error) {
	if provider == "" {
		if len(items) == 0 {
			return money.Money{}, fmt.Errorf("shipping needs order items")
		}
		return money.New(0, items[0].Total.Currency)
	}
	if s.carriers == nil {
		return money.Money{}, fmt.Errorf("delivery quotes are not configured")
	}
	optionCode = strings.TrimSpace(optionCode)
	if optionCode == "" {
		return money.Money{}, fmt.Errorf("delivery option code is required")
	}
	carrier, ok := s.carriers.Get(provider)
	if !ok {
		return money.Money{}, fmt.Errorf("delivery carrier %q is not enabled", provider)
	}
	shipmentItems := make([]deliveryDomain.ShipmentItem, 0, len(items))
	for _, item := range items {
		if item.VariantID == nil {
			return money.Money{}, fmt.Errorf("delivery item has no variant")
		}
		shipmentItems = append(shipmentItems, deliveryDomain.ShipmentItem{VariantID: *item.VariantID, Quantity: item.Quantity, WeightGrams: item.UnitWeightGrams})
	}
	options, err := carrier.Quote(ctx, deliveryDomain.ShipmentQuoteRequest{Destination: deliveryAddress(*details), Items: shipmentItems, Currency: items[0].Total.Currency})
	if err != nil {
		return money.Money{}, fmt.Errorf("quote delivery: %w", err)
	}
	for _, option := range options {
		if option.Code == optionCode {
			return option.Amount, nil
		}
	}
	return money.Money{}, fmt.Errorf("delivery option %q is unavailable", optionCode)
}

func deliveryAddress(details checkoutDomain.DeliveryDetails) deliveryDomain.Address {
	return deliveryDomain.Address{RecipientName: details.RecipientName, RecipientPhone: details.RecipientPhone, CountryCode: details.CountryCode, PostalCode: details.PostalCode, City: details.City, Line1: details.Line1, Line2: details.Line2, LocalityID: details.LocalityID, ServicePointID: details.ServicePointID}
}

func mapDelivery(details *checkoutDomain.DeliveryDetails) *ordersDomain.DeliveryDetails {
	if details == nil {
		return nil
	}
	return &ordersDomain.DeliveryDetails{RecipientName: details.RecipientName, RecipientPhone: details.RecipientPhone, CountryCode: details.CountryCode, PostalCode: details.PostalCode, City: details.City, Line1: details.Line1, Line2: details.Line2, LocalityID: details.LocalityID, ServicePointID: details.ServicePointID}
}

func (s *Service) ConfirmPayment(ctx context.Context, confirmation workflowDomain.PaymentConfirmation) error {
	if s.workflow == nil {
		return fmt.Errorf("checkout payment workflow is not configured")
	}
	return s.workflow.MarkPaid(ctx, confirmation)
}

func (s *Service) CancelPayment(ctx context.Context, orderID uuid.UUID) error {
	if s.workflow == nil {
		return fmt.Errorf("checkout payment workflow is not configured")
	}
	return s.workflow.CancelPending(ctx, orderID)
}

func (s *Service) releasePrepared(ctx context.Context, reservationIDs []uuid.UUID) {
	for _, reservationID := range reservationIDs {
		_ = s.inventory.Release(context.WithoutCancel(ctx), reservationID)
	}
}

func (s *Service) snapshotItems(ctx context.Context, quantities map[uuid.UUID]int, locale string) ([]ordersDomain.Item, money.Money, error) {
	variantIDs := make([]uuid.UUID, 0, len(quantities))
	for variantID := range quantities {
		variantIDs = append(variantIDs, variantID)
	}
	sort.Slice(variantIDs, func(i, j int) bool { return variantIDs[i].String() < variantIDs[j].String() })

	items := make([]ordersDomain.Item, 0, len(variantIDs))
	var subtotal money.Money
	for index, variantID := range variantIDs {
		variant, err := s.variants.FindActiveForCheckout(ctx, variantID, locale)
		if err != nil {
			return nil, money.Money{}, err
		}
		quantity := quantities[variantID]
		if variant.UnitPrice.Amount > math.MaxInt64/int64(quantity) {
			return nil, money.Money{}, fmt.Errorf("checkout line total overflows")
		}
		lineTotal, err := money.New(variant.UnitPrice.Amount*int64(quantity), variant.UnitPrice.Currency)
		if err != nil {
			return nil, money.Money{}, err
		}
		if index == 0 {
			subtotal = lineTotal
		} else if subtotal, err = subtotal.Add(lineTotal); err != nil {
			return nil, money.Money{}, err
		}
		items = append(items, ordersDomain.Item{VariantID: &variant.VariantID, ProductName: variant.ProductName, SKU: variant.SKU, Quantity: quantity, UnitPrice: variant.UnitPrice, Total: lineTotal, UnitWeightGrams: variant.WeightGrams})
	}
	return items, subtotal, nil
}
