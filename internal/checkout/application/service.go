package application

import (
	"context"
	"fmt"
	"sort"
	"time"

	checkoutDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/checkout/domain"
	inventoryDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/inventory/domain"
	"github.com/google/uuid"
)

type Service struct{ inventory inventoryDomain.Service }

func NewService(inventory inventoryDomain.Service) *Service { return &Service{inventory: inventory} }

func (s *Service) PreparePayment(ctx context.Context, request checkoutDomain.PrepareRequest) (*checkoutDomain.PreparedCheckout, error) {
	if request.CheckoutID == uuid.Nil || len(request.Lines) == 0 || !request.ExpiresAt.After(time.Now()) {
		return nil, fmt.Errorf("invalid checkout preparation")
	}
	type key struct{ variant, warehouse uuid.UUID }
	aggregated := map[key]int{}
	for _, line := range request.Lines {
		if line.VariantID == uuid.Nil || line.WarehouseID == uuid.Nil || line.Quantity <= 0 {
			return nil, fmt.Errorf("invalid checkout line")
		}
		aggregated[key{line.VariantID, line.WarehouseID}] += line.Quantity
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
	result := &checkoutDomain.PreparedCheckout{CheckoutID: request.CheckoutID, ExpiresAt: request.ExpiresAt, ReservationIDs: make([]uuid.UUID, 0, len(reservations))}
	for _, reservation := range reservations {
		result.ReservationIDs = append(result.ReservationIDs, reservation.ID)
	}
	return result, nil
}
