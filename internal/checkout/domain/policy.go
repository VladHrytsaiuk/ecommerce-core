package domain

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
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
	// ErrRedirectNotAllowed refuses a return or cancel address outside the
	// store's own origins.
	ErrRedirectNotAllowed = errors.New("checkout redirect address is not one of the store's origins")
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

// The request fields a refused redirect is reported against.
const (
	RedirectFieldReturn = "return_url"
	RedirectFieldCancel = "cancel_url"
)

// RedirectNotAllowedError names the address that was refused. A 422 that said
// only "one or more fields are invalid" left an integrator guessing between the
// return and the cancel address.
type RedirectNotAllowedError struct{ Field string }

func (e *RedirectNotAllowedError) Error() string {
	return fmt.Sprintf("%s: %s", ErrRedirectNotAllowed, e.Field)
}
func (e *RedirectNotAllowedError) Unwrap() error { return ErrRedirectNotAllowed }

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
	// AllowedRedirectOrigins are the only origins a payment provider may send
	// the buyer back to. Each entry is compared as an origin, so any absolute
	// http(s) address of the storefront works; Bootstrap still passes them
	// through NormalizeRedirectOrigins to drop invalid and duplicate entries.
	AllowedRedirectOrigins []string
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

// ValidateRedirectURL accepts an empty address, which leaves the provider on its
// own result page, or one whose origin is among AllowedRedirectOrigins.
//
// The address used to go to the payment provider exactly as the client sent it.
// A guest checkout created with someone else's return address, and its payment
// link sent to a victim, took the victim through the store's genuine payment
// page and then delivered them to that address — carrying the trust the real
// page had just earned.
func (p Policy) ValidateRedirectURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	origin, ok := redirectOrigin(raw)
	if !ok {
		return ErrRedirectNotAllowed
	}
	for _, allowed := range p.AllowedRedirectOrigins {
		// Normalised here, not only at composition. A policy assembled by hand
		// with "https://Shop.Example/" instead of the normalised form would
		// otherwise refuse every legitimate address — failing closed, but with a
		// 422 that points nowhere near the cause.
		if normalized, ok := redirectOrigin(strings.TrimSpace(allowed)); ok && normalized == origin {
			return nil
		}
	}
	return ErrRedirectNotAllowed
}

// NormalizeRedirectOrigins reduces configured addresses to comparable origins.
// Anything that is not a usable absolute http(s) address is dropped rather than
// allowed, so a malformed entry can only ever narrow the list.
func NormalizeRedirectOrigins(addresses []string) []string {
	origins := make([]string, 0, len(addresses))
	seen := make(map[string]struct{}, len(addresses))
	for _, address := range addresses {
		origin, ok := redirectOrigin(strings.TrimSpace(address))
		if !ok {
			continue
		}
		if _, duplicate := seen[origin]; duplicate {
			continue
		}
		seen[origin] = struct{}{}
		origins = append(origins, origin)
	}
	return origins
}

// redirectOrigin returns scheme://host[:port] in lower case, with the scheme's
// default port removed so https://shop.example and https://shop.example:443
// compare equal. It refuses whatever a browser could resolve somewhere other
// than where it appears to point: a relative or scheme-relative address, a
// non-http(s) scheme, user info, or an empty host.
func redirectOrigin(raw string) (string, bool) {
	if raw == "" {
		return "", false
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Opaque != "" || parsed.User != nil || parsed.Hostname() == "" {
		return "", false
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", false
	}
	host, port := strings.ToLower(parsed.Hostname()), parsed.Port()
	if (scheme == "https" && port == "443") || (scheme == "http" && port == "80") {
		port = ""
	}
	if port != "" {
		host = net.JoinHostPort(host, port)
	} else if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	return scheme + "://" + host, true
}
