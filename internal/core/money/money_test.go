package money

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestNewNormalizesCurrencyAndRejectsInvalidValues(t *testing.T) {
	value, err := NewMoney(1299, " eur ")
	if err != nil || value.Amount() != 1299 || value.Currency() != "EUR" {
		t.Fatalf("NewMoney() = %+v, %v", value, err)
	}
	if _, err := NewMoney(-1, "EUR"); err == nil {
		t.Fatal("NewMoney() accepted a negative amount")
	} else if !errors.Is(err, ErrInvalidMoney) {
		t.Fatalf("NewMoney() error = %v, want ErrInvalidMoney", err)
	}
	if _, err := NewMoney(1, "EURO"); err == nil {
		t.Fatal("NewMoney() accepted an invalid currency")
	}
	if _, err := NewMoney(1, "ZZZ"); err == nil {
		t.Fatal("NewMoney() accepted an unknown ISO-4217 currency")
	}
}

func TestAddRequiresSameCurrency(t *testing.T) {
	eur, _ := NewMoney(100, "EUR")
	otherEUR, _ := NewMoney(25, "EUR")
	result, err := eur.Add(otherEUR)
	if err != nil || result.Amount() != 125 {
		t.Fatalf("Add() = %+v, %v", result, err)
	}
	uah, _ := NewMoney(1, "UAH")
	if _, err := eur.Add(uah); err == nil {
		t.Fatal("Add() accepted different currencies")
	} else if !errors.Is(err, ErrCurrencyMismatch) {
		t.Fatalf("Add() error = %v, want ErrCurrencyMismatch", err)
	}
}

func TestUnmarshalJSONUsesConstructorValidation(t *testing.T) {
	var value Money
	if err := json.Unmarshal([]byte(`{"amount":125,"currency":"eur"}`), &value); err != nil {
		t.Fatalf("UnmarshalJSON() error = %v", err)
	}
	if value.Amount() != 125 || value.Currency() != "EUR" {
		t.Fatalf("UnmarshalJSON() = %d %q", value.Amount(), value.Currency())
	}
	if err := json.Unmarshal([]byte(`{"amount":-1,"currency":"EUR"}`), &value); err == nil {
		t.Fatal("UnmarshalJSON() accepted a negative amount")
	}
	before := value
	for _, payload := range []string{`null`, `{}`} {
		if err := json.Unmarshal([]byte(payload), &value); err == nil {
			t.Fatalf("UnmarshalJSON(%s) error = nil", payload)
		}
		if value != before {
			t.Fatalf("UnmarshalJSON(%s) modified receiver on error", payload)
		}
	}
	for _, payload := range []string{
		`{"Amount":1,"amount":2,"Currency":"EUR"}`,
		`{"Amount":1,"Currency":"EUR","extra":true}`,
		`{"Amount":1,"Currency":"EUR"} {}`,
	} {
		if err := json.Unmarshal([]byte(payload), &value); err == nil {
			t.Fatalf("UnmarshalJSON(%s) error = nil", payload)
		}
		if value != before {
			t.Fatalf("UnmarshalJSON(%s) modified receiver on error", payload)
		}
	}
}

func TestJSONPreservesLegacyWireContractAndAcceptsBothCases(t *testing.T) {
	value, err := NewMoney(125, "EUR")
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(encoded), `{"Amount":125,"Currency":"EUR"}`; got != want {
		t.Fatalf("MarshalJSON() = %s, want %s", got, want)
	}
	for _, payload := range []string{`{"Amount":125,"Currency":"EUR"}`, `{"amount":125,"currency":"EUR"}`} {
		var decoded Money
		if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
			t.Fatalf("UnmarshalJSON(%s) error = %v", payload, err)
		}
		if decoded != value {
			t.Fatalf("UnmarshalJSON(%s) = %+v, want %+v", payload, decoded, value)
		}
	}
}
