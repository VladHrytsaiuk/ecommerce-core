package money

import (
	"fmt"
	"regexp"
	"strings"
)

var currencyCode = regexp.MustCompile(`^[A-Z]{3}$`)

type Money struct {
	Amount   int64
	Currency string
}

func New(amount int64, currency string) (Money, error) {
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if amount < 0 || !currencyCode.MatchString(currency) {
		return Money{}, fmt.Errorf("invalid money")
	}
	return Money{Amount: amount, Currency: currency}, nil
}

func (m Money) Add(other Money) (Money, error) {
	if m.Currency != other.Currency {
		return Money{}, fmt.Errorf("currency mismatch")
	}
	return New(m.Amount+other.Amount, m.Currency)
}
