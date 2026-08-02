package tax

import (
	"testing"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
)

func TestPolicyCalculate(t *testing.T) {
	tests := []struct {
		name                  string
		mode                  Mode
		rate                  int
		amount                int64
		wantSubtotal, wantTax int64
		wantTotal             int64
	}{
		{name: "no tax", mode: ModeNone, amount: 1000, wantSubtotal: 1000, wantTax: 0, wantTotal: 1000},
		{name: "VAT excluded", mode: ModeVATExcluded, rate: 20, amount: 1000, wantSubtotal: 1000, wantTax: 200, wantTotal: 1200},
		{name: "VAT included", mode: ModeVATIncluded, rate: 20, amount: 1200, wantSubtotal: 1000, wantTax: 200, wantTotal: 1200},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy, err := NewPolicy(tt.mode, tt.rate)
			if err != nil {
				t.Fatalf("NewPolicy() error = %v", err)
			}
			amount, _ := money.New(tt.amount, "EUR")
			got, err := policy.Calculate(amount)
			if err != nil {
				t.Fatalf("Calculate() error = %v", err)
			}
			if got.Subtotal.Amount != tt.wantSubtotal || got.Tax.Amount != tt.wantTax || got.Total.Amount != tt.wantTotal || got.Total.Currency != "EUR" {
				t.Fatalf("Calculate() = %+v", got)
			}
		})
	}
}

func TestNewPolicyRejectsInvalidConfiguration(t *testing.T) {
	if _, err := NewPolicy(ModeVATExcluded, 101); err == nil {
		t.Fatal("NewPolicy() error = nil, want validation error")
	}
	if _, err := NewPolicy(ModeNone, 1); err == nil {
		t.Fatal("NewPolicy() error = nil, want validation error")
	}
}
