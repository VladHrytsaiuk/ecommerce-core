package money

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"strings"

	"golang.org/x/text/currency"
)

var currencyCode = regexp.MustCompile(`^[A-Z]{3}$`)

var (
	ErrInvalidMoney       = errors.New("invalid money")
	ErrCurrencyMismatch   = errors.New("money currency mismatch")
	ErrAmountOverflow     = errors.New("money addition overflow")
	ErrNegativeResult     = errors.New("money subtraction would be negative")
	ErrNullJSON           = errors.New("money must not be null")
	ErrNilJSONDestination = errors.New("money destination is nil")
)

type Money struct {
	amount   int64
	currency string
}

func NewMoney(amount int64, currency string) (Money, error) {
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if amount < 0 || !isISO4217Currency(currency) {
		return Money{}, fmt.Errorf("%w: amount must be non-negative and currency must be ISO-4217", ErrInvalidMoney)
	}
	return Money{amount: amount, currency: currency}, nil
}

func (m Money) Amount() int64    { return m.amount }
func (m Money) Currency() string { return m.currency }

// IsZero reports whether the monetary amount is zero. It intentionally does
// not claim that a zero-value Money is valid; callers requiring a real value
// must still call Validate or receive it from NewMoney.
func (m Money) IsZero() bool { return m.amount == 0 }

func (m Money) Validate() error {
	if m.amount < 0 || !isISO4217Currency(m.currency) {
		return ErrInvalidMoney
	}
	return nil
}

func isISO4217Currency(code string) bool {
	if !currencyCode.MatchString(code) {
		return false
	}
	unit, err := currency.ParseISO(code)
	return err == nil && unit != currency.XXX
}

func (m Money) Add(other Money) (Money, error) {
	if err := m.Validate(); err != nil {
		return Money{}, err
	}
	if err := other.Validate(); err != nil {
		return Money{}, err
	}
	if m.currency != other.currency {
		return Money{}, ErrCurrencyMismatch
	}
	if m.amount > math.MaxInt64-other.amount {
		return Money{}, ErrAmountOverflow
	}
	return NewMoney(m.amount+other.amount, m.currency)
}

func (m Money) Subtract(other Money) (Money, error) {
	if err := m.Validate(); err != nil {
		return Money{}, err
	}
	if err := other.Validate(); err != nil {
		return Money{}, err
	}
	if m.currency != other.currency {
		return Money{}, ErrCurrencyMismatch
	}
	if other.amount > m.amount {
		return Money{}, ErrNegativeResult
	}
	return NewMoney(m.amount-other.amount, m.currency)
}

// MarshalJSON keeps Money explicit at transport boundaries while preventing
// callers from bypassing its constructor through exported struct fields.
func (m Money) MarshalJSON() ([]byte, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		// Preserve the original public wire contract. Money used exported Go
		// fields before it became opaque, therefore encoding/json emitted these
		// exact key names.
		Amount   int64  `json:"Amount"`
		Currency string `json:"Currency"`
	}{Amount: m.amount, Currency: m.currency})
}

func (m *Money) UnmarshalJSON(data []byte) error {
	if m == nil {
		return ErrNilJSONDestination
	}
	amount, currency, err := decodeJSON(data)
	if err != nil {
		return err
	}
	value, err := NewMoney(amount, currency)
	if err != nil {
		return err
	}
	*m = value
	return nil
}

// decodeJSON accepts the legacy PascalCase and lowercase form, but rejects
// duplicate, unknown and trailing fields. Financial inputs must not have
// ambiguous "last key wins" semantics.
func decodeJSON(data []byte) (int64, string, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	first, err := decoder.Token()
	if err != nil {
		return 0, "", fmt.Errorf("decode money: %w", err)
	}
	if first == nil {
		return 0, "", ErrNullJSON
	}
	if delimiter, ok := first.(json.Delim); !ok || delimiter != '{' {
		return 0, "", fmt.Errorf("decode money: expected object")
	}

	var amount int64
	var currency string
	seen := map[string]bool{}
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return 0, "", fmt.Errorf("decode money key: %w", err)
		}
		key, ok := keyToken.(string)
		if !ok {
			return 0, "", fmt.Errorf("decode money: object key is not a string")
		}
		key = strings.ToLower(key)
		if key != "amount" && key != "currency" {
			return 0, "", fmt.Errorf("decode money: unknown field %q", key)
		}
		if seen[key] {
			return 0, "", fmt.Errorf("decode money: duplicate field %q", key)
		}
		seen[key] = true
		switch key {
		case "amount":
			if err := decoder.Decode(&amount); err != nil {
				return 0, "", fmt.Errorf("decode money amount: %w", err)
			}
		case "currency":
			if err := decoder.Decode(&currency); err != nil {
				return 0, "", fmt.Errorf("decode money currency: %w", err)
			}
		}
	}
	if _, err := decoder.Token(); err != nil {
		return 0, "", fmt.Errorf("decode money: %w", err)
	}
	if !seen["amount"] || !seen["currency"] {
		return 0, "", fmt.Errorf("decode money: amount and currency are required")
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return 0, "", fmt.Errorf("decode money: trailing data")
		}
		return 0, "", fmt.Errorf("decode money: trailing data: %w", err)
	}
	return amount, currency, nil
}
