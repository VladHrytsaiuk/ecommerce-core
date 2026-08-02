package application

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	catalogDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/domain"
	checkoutDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/checkout/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/tax"
	inventoryDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/inventory/domain"
	ordersDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/orders/domain"
	"github.com/google/uuid"
)

type variantFinder interface {
	FindActiveForCheckout(context.Context, uuid.UUID, string) (*catalogDomain.CheckoutVariant, error)
}

type Service struct {
	inventory inventoryDomain.Service
	variants  variantFinder
	tax       tax.Calculator
}

func NewService(inventory inventoryDomain.Service, variants variantFinder, tax tax.Calculator) *Service {
	return &Service{inventory: inventory, variants: variants, tax: tax}
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
		items = append(items, ordersDomain.Item{VariantID: &variant.VariantID, ProductName: variant.ProductName, SKU: variant.SKU, Quantity: quantity, UnitPrice: variant.UnitPrice, Total: lineTotal})
	}
	return items, subtotal, nil
}
