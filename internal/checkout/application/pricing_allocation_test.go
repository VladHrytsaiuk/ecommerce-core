package application

import (
	"testing"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
	ordersDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/orders/domain"
)

func TestAllocateLineDiscountsUsesLargestRemainderDeterministically(t *testing.T) {
	items := []ordersDomain.Item{
		{Total: mustMoney(101, "UAH")},
		{Total: mustMoney(100, "UAH")},
		{Total: mustMoney(99, "UAH")},
	}
	if err := allocateLineDiscounts(items, mustMoney(100, "UAH")); err != nil {
		t.Fatalf("allocateLineDiscounts() error = %v", err)
	}
	got := []int64{items[0].Discount.Amount(), items[1].Discount.Amount(), items[2].Discount.Amount()}
	want := []int64{34, 33, 33}
	for index := range want {
		if got[index] != want[index] || items[index].Discount.Currency() != "UAH" {
			t.Fatalf("discounts = %v, want %v", got, want)
		}
	}
}

func mustMoney(amount int64, currency string) money.Money {
	value, err := money.NewMoney(amount, currency)
	if err != nil {
		panic(err)
	}
	return value
}
