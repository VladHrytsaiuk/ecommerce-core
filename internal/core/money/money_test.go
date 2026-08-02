package money

import "testing"

func TestNewNormalizesCurrencyAndRejectsInvalidValues(t *testing.T) {
	value, err := New(1299, " eur ")
	if err != nil || value.Amount != 1299 || value.Currency != "EUR" {
		t.Fatalf("New() = %+v, %v", value, err)
	}
	if _, err := New(-1, "EUR"); err == nil {
		t.Fatal("New() accepted a negative amount")
	}
	if _, err := New(1, "EURO"); err == nil {
		t.Fatal("New() accepted an invalid currency")
	}
}

func TestAddRequiresSameCurrency(t *testing.T) {
	eur, _ := New(100, "EUR")
	otherEUR, _ := New(25, "EUR")
	result, err := eur.Add(otherEUR)
	if err != nil || result.Amount != 125 {
		t.Fatalf("Add() = %+v, %v", result, err)
	}
	uah, _ := New(1, "UAH")
	if _, err := eur.Add(uah); err == nil {
		t.Fatal("Add() accepted different currencies")
	}
}
