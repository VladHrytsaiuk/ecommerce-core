package application

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	checkoutDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/checkout/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
	promosDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/promos/domain"
)

// PromoCalculatorDecorator applies a validated promotion before the configured
// tax calculator. The resulting snapshot is copied to the immutable order.
type PromoCalculatorDecorator struct {
	next   checkoutDomain.PriceCalculator
	promos promosDomain.Repository
	now    func() time.Time
}

func NewPromoCalculatorDecorator(next checkoutDomain.PriceCalculator, promos promosDomain.Repository) *PromoCalculatorDecorator {
	return &PromoCalculatorDecorator{next: next, promos: promos, now: func() time.Time { return time.Now().UTC() }}
}

func (c *PromoCalculatorDecorator) Calculate(ctx context.Context, request checkoutDomain.PriceCalculationRequest) (checkoutDomain.Price, error) {
	if c.next == nil || c.promos == nil {
		return checkoutDomain.Price{}, fmt.Errorf("promo price calculator is not configured")
	}
	code := strings.ToUpper(strings.TrimSpace(request.PromoCode))
	if code == "" {
		return c.next.Calculate(ctx, checkoutDomain.PriceCalculationRequest{Subtotal: request.Subtotal})
	}
	promo, err := c.promos.FindByCode(ctx, code)
	if err != nil {
		return checkoutDomain.Price{}, err
	}
	if !promo.IsActive {
		return checkoutDomain.Price{}, promosDomain.ErrCodeInactive
	}
	if promo.ValidUntil != nil && !promo.ValidUntil.After(c.now()) {
		return checkoutDomain.Price{}, promosDomain.ErrCodeExpired
	}
	discount, err := calculateDiscount(request.Subtotal, *promo)
	if err != nil {
		return checkoutDomain.Price{}, err
	}
	taxable, err := money.New(request.Subtotal.Amount-discount.Amount, request.Subtotal.Currency)
	if err != nil {
		return checkoutDomain.Price{}, err
	}
	price, err := c.next.Calculate(ctx, checkoutDomain.PriceCalculationRequest{Subtotal: taxable})
	if err != nil {
		return checkoutDomain.Price{}, err
	}
	price.Discount = discount
	price.Promotion = &checkoutDomain.PromotionSnapshot{Code: promo.Code, Type: promo.DiscountType, Value: promo.DiscountValue, Currency: promo.Currency, Discount: discount}
	return price, nil
}

func calculateDiscount(subtotal money.Money, promo promosDomain.Code) (money.Money, error) {
	var amount int64
	switch promo.DiscountType {
	case promosDomain.TypePercent:
		if promo.DiscountValue <= 0 || promo.DiscountValue > 10000 {
			return money.Money{}, promosDomain.ErrInvalidCode
		}
		// Discount values are basis points. This split avoids int64 overflow.
		whole := subtotal.Amount / 10000
		remainder := subtotal.Amount % 10000
		if whole > math.MaxInt64/promo.DiscountValue {
			return money.Money{}, promosDomain.ErrInvalidCode
		}
		amount = whole*promo.DiscountValue + (remainder*promo.DiscountValue+5000)/10000
	case promosDomain.TypeFixed:
		if promo.Currency != subtotal.Currency || promo.DiscountValue <= 0 {
			return money.Money{}, promosDomain.ErrInvalidCode
		}
		amount = promo.DiscountValue
	default:
		return money.Money{}, promosDomain.ErrInvalidCode
	}
	if amount > subtotal.Amount {
		amount = subtotal.Amount
	}
	return money.New(amount, subtotal.Currency)
}

var _ checkoutDomain.PriceCalculator = (*PromoCalculatorDecorator)(nil)
