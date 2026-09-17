package domain

import (
	"context"
	"fmt"
	"strings"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/tax"
)

// PromotionSnapshot preserves the promotion rule used at checkout.
type PromotionSnapshot struct {
	Code     string
	Type     string
	Value    int64
	Currency string
	Discount money.Money
}

type PriceCalculationRequest struct {
	Subtotal  money.Money
	PromoCode string
}
type Price struct {
	Subtotal  money.Money
	Discount  money.Money
	Tax       money.Money
	Total     money.Money
	Promotion *PromotionSnapshot
}

// PriceCalculator is a checkout extension point. Promotion decorators must
// reduce the taxable subtotal before delegating to this calculator.
type PriceCalculator interface {
	Calculate(context.Context, PriceCalculationRequest) (Price, error)
}

type CheckoutPriceCalculator struct{ tax tax.Calculator }

func NewCheckoutPriceCalculator(calculator tax.Calculator) (*CheckoutPriceCalculator, error) {
	if calculator == nil {
		return nil, fmt.Errorf("checkout tax calculator is required")
	}
	return &CheckoutPriceCalculator{tax: calculator}, nil
}

func (c *CheckoutPriceCalculator) Calculate(_ context.Context, request PriceCalculationRequest) (Price, error) {
	if strings.TrimSpace(request.PromoCode) != "" {
		return Price{}, fmt.Errorf("promo code is not supported by the configured price calculator")
	}
	breakdown, err := c.tax.Calculate(request.Subtotal)
	if err != nil {
		return Price{}, err
	}
	zero, err := money.NewMoney(0, breakdown.Total.Currency())
	if err != nil {
		return Price{}, err
	}
	return Price{Subtotal: breakdown.Subtotal, Discount: zero, Tax: breakdown.Tax, Total: breakdown.Total}, nil
}

var _ PriceCalculator = (*CheckoutPriceCalculator)(nil)
