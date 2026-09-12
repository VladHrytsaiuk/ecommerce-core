package domain

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

var (
	ErrGuestCheckoutDisabled         = errors.New("guest checkout is disabled")
	ErrEmailVerificationRequired     = errors.New("verified email is required for checkout")
	ErrPhoneVerificationRequired     = errors.New("verified phone is required for checkout")
	ErrVerificationReaderUnavailable = errors.New("customer verification reader is not configured")
	ErrProfileIncomplete             = errors.New("customer profile is incomplete")
	ErrProfileReaderUnavailable      = errors.New("customer profile reader is not configured")
)

// VerificationStatus is the deliberately small customer projection Checkout
// needs to enforce its policy. It must never contain credentials or PII.
type VerificationStatus struct {
	IsEmailVerified bool
	IsPhoneVerified bool
}

// CustomerVerificationReader is a cross-module read port. Identity owns its
// implementation; Checkout depends only on this contract, never on Identity
// tables or persistence models.
type CustomerVerificationReader interface {
	GetVerificationStatus(ctx context.Context, customerID uuid.UUID) (VerificationStatus, error)
}

// CustomerProfileReader exposes field presence only. Identity keeps profile
// values and metadata private; Checkout only decides whether configured fields
// have been supplied.
type CustomerProfileReader interface {
	GetAvailableProfileFields(ctx context.Context, customerID uuid.UUID) (map[string]bool, error)
}

// ProfileIncompleteError lets the HTTP adapter safely return the missing field
// names while errors.Is still recognizes ErrProfileIncomplete.
type ProfileIncompleteError struct{ MissingFields []string }

func (e *ProfileIncompleteError) Error() string {
	return fmt.Sprintf("%s: %s", ErrProfileIncomplete, strings.Join(e.MissingFields, ", "))
}
func (e *ProfileIncompleteError) Unwrap() error { return ErrProfileIncomplete }

// Policy carries only checkout rules; it deliberately does not expose the
// global StoreConfig to application use cases.
type Policy struct {
	AllowGuest            bool
	RequirePhone          bool
	RequireVerifiedEmail  bool
	RequireVerifiedPhone  bool
	RequiredProfileFields []string
	OrderNumberPrefix     string
	// SupportedDeliveryProviders is assembled from the enabled adapter codes in
	// Bootstrap. Checkout stores only the selected code, never an adapter or a
	// provider-specific delivery field.
	SupportedDeliveryProviders []string
	DefaultDeliveryProvider    string
}

// orderNumberDigits is how much of the checkout UUID the order number carries.
//
// It was eight, which is 32 bits, and orders.number is UNIQUE. By the birthday
// bound two of 50,000 checkouts share an eight-character prefix about a quarter
// of the time, and by 200,000 it is all but certain; each collision fails the
// losing checkout at order insert. Twelve is 48 bits, which puts the same risk
// past any volume this core is built for.
//
// The number stays a deterministic function of the checkout ID, but nothing
// depends on its shape: a retried checkout is recognised by the attempt row's
// idempotency key, not by this string.
const orderNumberDigits = 12

// orderNumberPrefixLimit keeps the assembled number inside orders.number's
// VARCHAR(64). STORE_CODE, which supplies the prefix, may be 64 characters on
// its own, so an unbounded prefix would overflow the column and fail every
// checkout in the store.
const orderNumberPrefixLimit = 64 - orderNumberDigits - 1

func (p Policy) OrderNumber(checkoutID uuid.UUID) string {
	prefix := strings.ToUpper(strings.TrimSpace(p.OrderNumberPrefix))
	if prefix == "" {
		prefix = "ORDER"
	}
	if len(prefix) > orderNumberPrefixLimit {
		prefix = prefix[:orderNumberPrefixLimit]
	}
	// The UUID's own hyphen sits at index 8, so slicing the formatted string
	// past that point would spend a character of the budget on a separator
	// rather than on entropy.
	digits := strings.ReplaceAll(checkoutID.String(), "-", "")
	return prefix + "-" + strings.ToUpper(digits[:orderNumberDigits])
}

func (p Policy) ValidateCustomer(customerID *uuid.UUID, phone string) error {
	if !p.AllowGuest && customerID == nil {
		return ErrGuestCheckoutDisabled
	}
	if p.RequirePhone && strings.TrimSpace(phone) == "" {
		return fmt.Errorf("customer phone is required")
	}
	return nil
}

// ResolveDeliveryProvider applies the store delivery policy before stock is
// reserved. This avoids persisting an arbitrary browser-supplied provider code
// on an order, while allowing a provider-free deployment to omit delivery.
func (p Policy) ResolveDeliveryProvider(requested string) (string, error) {
	requested = strings.ToLower(strings.TrimSpace(requested))
	if requested == "" {
		requested = strings.ToLower(strings.TrimSpace(p.DefaultDeliveryProvider))
	}
	if requested == "" {
		return "", nil
	}
	for _, provider := range p.SupportedDeliveryProviders {
		if strings.ToLower(strings.TrimSpace(provider)) == requested {
			return requested, nil
		}
	}
	return "", fmt.Errorf("delivery provider %q is not enabled", requested)
}
