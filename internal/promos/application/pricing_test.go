package application

import (
	"context"
	"testing"
	"time"

	checkoutDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/checkout/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/tax"
	ordersDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/orders/domain"
	promosDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/promos/domain"
	"github.com/google/uuid"
)

func TestPromoCalculatorDecoratorDiscountsBeforeVAT(t *testing.T) {
	base, err := checkoutDomain.NewCheckoutPriceCalculator(mustVAT(t, tax.ModeVATExcluded, 20))
	if err != nil {
		t.Fatal(err)
	}
	decorator := NewPromoCalculatorDecorator(base, &fakePromos{code: &promosDomain.Code{ID: uuid.New(), Code: "SAVE10", DiscountType: promosDomain.TypePercent, DiscountValue: 1000, IsActive: true}})
	subtotal, _ := money.New(1000, "EUR")
	price, err := decorator.Calculate(context.Background(), checkoutDomain.PriceCalculationRequest{Subtotal: subtotal, PromoCode: " save10 "})
	if err != nil {
		t.Fatalf("Calculate() error = %v", err)
	}
	if price.Discount.Amount != 100 || price.Subtotal.Amount != 900 || price.Tax.Amount != 180 || price.Total.Amount != 1080 || price.Promotion == nil || price.Promotion.Code != "SAVE10" || price.Promotion.Value != 1000 {
		t.Fatalf("price = %+v", price)
	}
}

func TestPromoCalculatorDecoratorRejectsExpiredCode(t *testing.T) {
	base, _ := checkoutDomain.NewCheckoutPriceCalculator(mustVAT(t, tax.ModeNone, 0))
	expired := time.Now().Add(-time.Minute)
	decorator := NewPromoCalculatorDecorator(base, &fakePromos{code: &promosDomain.Code{Code: "OLD", DiscountType: promosDomain.TypeFixed, DiscountValue: 100, Currency: "EUR", IsActive: true, ValidUntil: &expired}})
	subtotal, _ := money.New(1000, "EUR")
	if _, err := decorator.Calculate(context.Background(), checkoutDomain.PriceCalculationRequest{Subtotal: subtotal, PromoCode: "OLD"}); err != promosDomain.ErrCodeExpired {
		t.Fatalf("Calculate() error = %v, want expired", err)
	}
}

func mustVAT(t *testing.T, mode tax.Mode, rate int) *tax.Policy {
	t.Helper()
	policy, err := tax.NewPolicy(mode, rate)
	if err != nil {
		t.Fatal(err)
	}
	return policy
}

type fakePromos struct{ code *promosDomain.Code }

func (f *fakePromos) FindByCode(context.Context, string) (*promosDomain.Code, error) {
	if f.code == nil {
		return nil, promosDomain.ErrCodeNotFound
	}
	return f.code, nil
}
func (*fakePromos) Reserve(context.Context, uuid.UUID, ordersDomain.Promotion) error { return nil }
func (*fakePromos) Commit(context.Context, uuid.UUID) error                          { return nil }
func (*fakePromos) Release(context.Context, uuid.UUID) error                         { return nil }
