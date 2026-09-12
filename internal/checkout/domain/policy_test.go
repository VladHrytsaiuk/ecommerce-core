package domain

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

// The order number is written into orders.number, which is UNIQUE, so its
// entropy decides how often a checkout fails for no reason the buyer can see.

func TestTheOrderNumberCarriesEnoughOfTheCheckoutToStayUnique(t *testing.T) {
	policy := Policy{OrderNumberPrefix: "es"}
	number := policy.OrderNumber(uuid.MustParse("3f7a9b21-c4d0-4e5f-8a1b-2c3d4e5f6071"))

	const want = "ES-3F7A9B21C4D0"
	if number != want {
		t.Fatalf("OrderNumber() = %q, want %q", number, want)
	}
	// Twelve hex characters is 48 bits. Eight was 32, and at 50,000 checkouts
	// two of them shared a prefix about a quarter of the time.
	digits := number[strings.LastIndex(number, "-")+1:]
	if len(digits) != 12 {
		t.Fatalf("the number carries %d hex characters, want 12", len(digits))
	}
	// The UUID's own hyphen sits at index 8; spending a character of the
	// budget on it would leave only 44 bits.
	if strings.Contains(digits, "-") {
		t.Fatalf("digits = %q, want contiguous entropy", digits)
	}
}

func TestTheSameCheckoutAlwaysProducesTheSameNumber(t *testing.T) {
	// A retried checkout must not open a second order under a second number.
	policy := Policy{OrderNumberPrefix: "es"}
	checkoutID := uuid.New()

	if first, second := policy.OrderNumber(checkoutID), policy.OrderNumber(checkoutID); first != second {
		t.Fatalf("OrderNumber() = %q then %q for one checkout", first, second)
	}
}

func TestDifferentCheckoutsDoNotShareANumber(t *testing.T) {
	policy := Policy{OrderNumberPrefix: "es"}
	seen := make(map[string]struct{}, 20000)
	for range 20000 {
		number := policy.OrderNumber(uuid.New())
		if _, exists := seen[number]; exists {
			t.Fatalf("two of 20,000 checkouts produced %q", number)
		}
		seen[number] = struct{}{}
	}
}

func TestTheNumberFitsItsColumnEvenWithTheLongestStoreCode(t *testing.T) {
	// STORE_CODE is validated as up to 64 characters, and orders.number is
	// VARCHAR(64). An unbounded prefix would overflow the column and fail
	// every checkout in the store.
	policy := Policy{OrderNumberPrefix: strings.Repeat("a", 64)}

	number := policy.OrderNumber(uuid.New())

	if len(number) > 64 {
		t.Fatalf("OrderNumber() is %d characters, want at most 64", len(number))
	}
	if !strings.HasPrefix(number, strings.Repeat("A", 51)+"-") {
		t.Fatalf("OrderNumber() = %q, want the prefix truncated rather than the digits", number)
	}
}

func TestAnAbsentPrefixFallsBackToAReadableOne(t *testing.T) {
	for _, prefix := range []string{"", "   "} {
		number := Policy{OrderNumberPrefix: prefix}.OrderNumber(uuid.New())
		if !strings.HasPrefix(number, "ORDER-") {
			t.Fatalf("OrderNumber() = %q, want the ORDER- fallback", number)
		}
	}
}

func TestGuestCheckoutAndPhoneAreEnforcedByPolicy(t *testing.T) {
	customerID := uuid.New()
	if err := (Policy{AllowGuest: false}).ValidateCustomer(nil, ""); err == nil {
		t.Fatal("ValidateCustomer() allowed a guest where guest checkout is disabled")
	}
	if err := (Policy{AllowGuest: true}).ValidateCustomer(nil, ""); err != nil {
		t.Fatalf("ValidateCustomer() error = %v, want a guest to be allowed", err)
	}
	if err := (Policy{AllowGuest: true, RequirePhone: true}).ValidateCustomer(&customerID, "  "); err == nil {
		t.Fatal("ValidateCustomer() accepted a blank phone where one is required")
	}
	if err := (Policy{AllowGuest: true, RequirePhone: true}).ValidateCustomer(&customerID, "+34123456789"); err != nil {
		t.Fatalf("ValidateCustomer() error = %v", err)
	}
}

func TestOnlyAnEnabledDeliveryProviderIsAccepted(t *testing.T) {
	// The requested code arrives from the browser and is persisted on the
	// order, so an unlisted one must never be stored.
	policy := Policy{SupportedDeliveryProviders: []string{"novaposhta", "DHLExpress"}, DefaultDeliveryProvider: "novaposhta"}

	for request, want := range map[string]string{
		"novaposhta":    "novaposhta",
		"  NovaPoshta ": "novaposhta",
		"dhlexpress":    "dhlexpress",
		"":              "novaposhta", // falls back to the store default
	} {
		got, err := policy.ResolveDeliveryProvider(request)
		if err != nil || got != want {
			t.Fatalf("ResolveDeliveryProvider(%q) = (%q, %v), want %q", request, got, err, want)
		}
	}
	if _, err := policy.ResolveDeliveryProvider("madeup"); err == nil {
		t.Fatal("ResolveDeliveryProvider() accepted a provider that is not enabled")
	}
	// A deployment with no delivery at all stores no provider rather than failing.
	if got, err := (Policy{}).ResolveDeliveryProvider(""); err != nil || got != "" {
		t.Fatalf("ResolveDeliveryProvider() = (%q, %v) with no providers configured", got, err)
	}
}
