package novaposhta

import "github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"

func mustMoney(amount int64, currency string) money.Money {
	value, err := money.NewMoney(amount, currency)
	if err != nil {
		panic(err)
	}
	return value
}
