// Package tax contains provider-neutral tax calculation policies.
package tax

import (
	"fmt"
	"math"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
)

type Mode string

const (
	ModeNone        Mode = "none"
	ModeVATIncluded Mode = "vat_included"
	ModeVATExcluded Mode = "vat_excluded"
)

// Breakdown uses the order convention: Subtotal is before tax, Tax is the tax
// amount, and Total is the amount charged to the customer.
type Breakdown struct {
	Subtotal money.Money
	Tax      money.Money
	Total    money.Money
}

// Calculator is a checkout policy. It deliberately has no provider, HTTP, or
// database dependency.
type Calculator interface {
	Calculate(money.Money) (Breakdown, error)
}

// Policy currently supports one configured VAT percentage. More advanced tax
// engines can implement Calculator without changing checkout or orders.
type Policy struct {
	mode    Mode
	vatRate int
}

func NewPolicy(mode Mode, vatRate int) (*Policy, error) {
	switch mode {
	case ModeNone:
		if vatRate != 0 {
			return nil, fmt.Errorf("VAT_RATE must be zero when TAX_MODE is none")
		}
	case ModeVATIncluded, ModeVATExcluded:
		if vatRate < 0 || vatRate > 100 {
			return nil, fmt.Errorf("VAT_RATE must be between 0 and 100")
		}
	default:
		return nil, fmt.Errorf("unsupported TAX_MODE %q", mode)
	}

	return &Policy{mode: mode, vatRate: vatRate}, nil
}

func (p *Policy) Calculate(amount money.Money) (Breakdown, error) {
	if _, err := money.New(amount.Amount, amount.Currency); err != nil {
		return Breakdown{}, err
	}

	switch p.mode {
	case ModeNone:
		zero, _ := money.New(0, amount.Currency)
		return Breakdown{Subtotal: amount, Tax: zero, Total: amount}, nil
	case ModeVATExcluded:
		taxAmount, err := roundedPercent(amount.Amount, int64(p.vatRate), 100)
		if err != nil {
			return Breakdown{}, err
		}
		taxValue, _ := money.New(taxAmount, amount.Currency)
		total, err := amount.Add(taxValue)
		if err != nil {
			return Breakdown{}, err
		}
		return Breakdown{Subtotal: amount, Tax: taxValue, Total: total}, nil
	case ModeVATIncluded:
		taxAmount, err := roundedPercent(amount.Amount, int64(p.vatRate), 100+int64(p.vatRate))
		if err != nil {
			return Breakdown{}, err
		}
		taxValue, _ := money.New(taxAmount, amount.Currency)
		subtotal, err := money.New(amount.Amount-taxAmount, amount.Currency)
		if err != nil {
			return Breakdown{}, err
		}
		return Breakdown{Subtotal: subtotal, Tax: taxValue, Total: amount}, nil
	default:
		return Breakdown{}, fmt.Errorf("unsupported TAX_MODE %q", p.mode)
	}
}

func roundedPercent(amount, numerator, denominator int64) (int64, error) {
	if denominator <= 0 || numerator < 0 || amount < 0 {
		return 0, fmt.Errorf("tax calculation overflow")
	}
	if numerator != 0 && amount > (math.MaxInt64-denominator/2)/numerator {
		return 0, fmt.Errorf("tax calculation overflow")
	}
	return (amount*numerator + denominator/2) / denominator, nil
}

var _ Calculator = (*Policy)(nil)
