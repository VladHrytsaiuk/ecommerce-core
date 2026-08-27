package application

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
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

func (s *Service) ListByCustomer(ctx context.Context, customerID uuid.UUID, page, limit int) (domain.Page, error) {
	if customerID == uuid.Nil || page < 1 || limit < 1 {
		return domain.Page{}, fmt.Errorf("invalid order list request")
	}
	return s.repo.ListByCustomer(ctx, customerID, page, limit)
}

// NewPendingOrder validates the immutable commercial snapshot before any
// persistence adapter or cross-module workflow receives it.
func NewPendingOrder(draft domain.Draft) (*domain.Order, error) {
	if strings.TrimSpace(draft.Number) == "" || len(draft.Items) == 0 {
		return nil, fmt.Errorf("invalid order draft")
	}
	if !draft.ExpiresAt.IsZero() && !draft.ExpiresAt.After(time.Now()) {
		return nil, fmt.Errorf("order expiry must be in the future")
	}
	if draft.Shipping.IsZero() && draft.Shipping.Currency() == "" {
		shipping, err := money.NewMoney(0, draft.Subtotal.Currency())
		if err != nil {
			return nil, err
		}
		draft.Shipping = shipping
	}
	if draft.Subtotal.Validate() != nil || draft.Tax.Validate() != nil || draft.Shipping.Validate() != nil || draft.Total.Validate() != nil || draft.Subtotal.Currency() != draft.Tax.Currency() || draft.Subtotal.Currency() != draft.Shipping.Currency() || draft.Subtotal.Currency() != draft.Total.Currency() {
		return nil, fmt.Errorf("invalid order totals")
	}
	withTax, err := draft.Subtotal.Add(draft.Tax)
	if err != nil {
		return nil, fmt.Errorf("invalid order totals: %w", err)
	}
	withShipping, err := withTax.Add(draft.Shipping)
	if err != nil || withShipping.Amount() != draft.Total.Amount() {
		return nil, fmt.Errorf("invalid order totals")
	}
	var sum, allocatedDiscount int64
	for index := range draft.Items {
		item := &draft.Items[index]
		if item.Discount.IsZero() && item.Discount.Currency() == "" {
			discount, err := money.NewMoney(0, draft.Total.Currency())
			if err != nil {
				return nil, err
			}
			item.Discount = discount
		}
		if item.Quantity <= 0 || item.ProductName == "" || item.UnitWeightGrams < 0 || item.UnitPrice.Validate() != nil || item.Total.Validate() != nil || item.Discount.Validate() != nil || item.UnitPrice.Currency() != draft.Total.Currency() || item.Total.Currency() != draft.Total.Currency() || item.Discount.Currency() != draft.Total.Currency() || item.Discount.Amount() > item.Total.Amount() || item.UnitPrice.Amount() > 0 && item.Quantity > 0 && item.UnitPrice.Amount() > maxInt64/int64(item.Quantity) || item.UnitPrice.Amount()*int64(item.Quantity) != item.Total.Amount() {
			return nil, fmt.Errorf("invalid order item")
		}
		if sum > maxInt64-item.Total.Amount() || allocatedDiscount > maxInt64-item.Discount.Amount() {
			return nil, fmt.Errorf("order item totals overflow")
		}
		sum += item.Total.Amount()
		allocatedDiscount += item.Discount.Amount()
	}
	// Item totals are the immutable catalog-price snapshot. With VAT-included
	// pricing and/or a promotion the taxable subtotal can legitimately be lower
	// than this pre-tax, pre-discount item sum.
	if sum < draft.Subtotal.Amount() {
		return nil, fmt.Errorf("order subtotal exceeds item snapshot total")
	}
	if draft.Promotion != nil {
		if strings.TrimSpace(draft.Promotion.Code) == "" || strings.TrimSpace(draft.Promotion.Type) == "" || draft.Promotion.Value <= 0 || draft.Promotion.Discount.Validate() != nil || draft.Promotion.Discount.Currency() != draft.Total.Currency() {
			return nil, fmt.Errorf("invalid order promotion")
		}
		if allocatedDiscount != draft.Promotion.Discount.Amount() {
			return nil, fmt.Errorf("order line discounts do not match promotion")
		}
	} else if allocatedDiscount != 0 {
		return nil, fmt.Errorf("order line discounts require a promotion")
	}
	if draft.Contact != nil && (strings.TrimSpace(draft.Contact.Email) == "" || strings.TrimSpace(draft.Contact.Locale) == "") {
		return nil, fmt.Errorf("invalid order contact")
	}
	return &domain.Order{ID: uuid.New(), CartID: draft.CartID, Number: draft.Number, CustomerID: draft.CustomerID, Status: domain.StatusPendingPayment, Subtotal: draft.Subtotal, Tax: draft.Tax, Shipping: draft.Shipping, Total: draft.Total, PaymentProvider: draft.PaymentProvider, DeliveryProvider: draft.DeliveryProvider, Delivery: draft.Delivery, Items: draft.Items, Promotion: draft.Promotion, Contact: draft.Contact, ExpiresAt: draft.ExpiresAt}, nil
}

const maxInt64 = int64(^uint64(0) >> 1)
