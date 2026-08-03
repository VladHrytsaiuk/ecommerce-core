package application

import (
	"context"
	"fmt"
	"strings"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/orders/domain"
	"github.com/google/uuid"
)

type Service struct{ repo domain.Repository }

func NewService(repo domain.Repository) *Service { return &Service{repo: repo} }

func (s *Service) Create(ctx context.Context, draft domain.Draft) (*domain.Order, error) {
	order, err := NewPendingOrder(draft)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, order); err != nil {
		return nil, err
	}
	return order, nil
}

// NewPendingOrder validates the immutable commercial snapshot before any
// persistence adapter or cross-module workflow receives it.
func NewPendingOrder(draft domain.Draft) (*domain.Order, error) {
	if strings.TrimSpace(draft.Number) == "" || len(draft.Items) == 0 {
		return nil, fmt.Errorf("invalid order draft")
	}
	if draft.Shipping.Currency == "" && draft.Shipping.Amount == 0 {
		draft.Shipping.Currency = draft.Subtotal.Currency
	}
	if draft.Subtotal.Currency == "" || draft.Subtotal.Currency != draft.Tax.Currency || draft.Subtotal.Currency != draft.Shipping.Currency || draft.Subtotal.Currency != draft.Total.Currency || draft.Shipping.Amount < 0 || draft.Subtotal.Amount+draft.Tax.Amount+draft.Shipping.Amount != draft.Total.Amount {
		return nil, fmt.Errorf("invalid order totals")
	}
	var sum int64
	for _, item := range draft.Items {
		if item.Quantity <= 0 || item.ProductName == "" || item.UnitWeightGrams < 0 || item.UnitPrice.Currency != draft.Total.Currency || item.Total.Currency != draft.Total.Currency || item.UnitPrice.Amount*int64(item.Quantity) != item.Total.Amount {
			return nil, fmt.Errorf("invalid order item")
		}
		sum += item.Total.Amount
	}
	if sum != draft.Subtotal.Amount {
		return nil, fmt.Errorf("order subtotal does not match items")
	}
	return &domain.Order{ID: uuid.New(), CartID: draft.CartID, Number: draft.Number, CustomerID: draft.CustomerID, Status: domain.StatusPendingPayment, Subtotal: draft.Subtotal, Tax: draft.Tax, Shipping: draft.Shipping, Total: draft.Total, PaymentProvider: draft.PaymentProvider, DeliveryProvider: draft.DeliveryProvider, Delivery: draft.Delivery, Items: draft.Items}, nil
}
