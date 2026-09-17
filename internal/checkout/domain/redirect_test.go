package domain

import (
	"errors"
	"testing"
)

func TestARedirectMustReturnToOneOfTheStoresOrigins(t *testing.T) {
	policy := Policy{AllowedRedirectOrigins: NormalizeRedirectOrigins([]string{"https://shop.example", "http://localhost:3000"})}

	for name, address := range map[string]string{
		"no address at all":        "",
		"the store with a path":    "https://shop.example/checkout/done?order=42#top",
		"the default port spelled": "https://shop.example:443/done",
		"host in a different case": "https://SHOP.example/done",
		"the local storefront":     "http://localhost:3000/done",
	} {
		t.Run("accepts "+name, func(t *testing.T) {
			if err := policy.ValidateRedirectURL(address); err != nil {
				t.Fatalf("ValidateRedirectURL(%q) = %v, want accepted", address, err)
			}
		})
	}

	for name, address := range map[string]string{
		"another site":                 "https://evil.example/fake-store",
		"a lookalike subdomain":        "https://shop.example.evil.example/done",
		"a store subdomain not listed": "https://pay.shop.example/done",
		"the right host over http":     "http://shop.example/done",
		"a different port":             "https://shop.example:8443/done",
		"a script":                     "javascript:alert(1)",
		"scheme-relative":              "//evil.example/done",
		"relative":                     "/done",
		// Two different guards: the first is refused by its host, which is
		// evil.example; the second has the store's host and is refused only
		// because user info never belongs in a return address.
		"a host hidden behind user info": "https://shop.example@evil.example/done",
		"user info before the store":     "https://buyer@shop.example/done",
		"a backslash trick":              `https://shop.example\@evil.example/done`,
		"not an address":                 "shop.example",
	} {
		t.Run("refuses "+name, func(t *testing.T) {
			if err := policy.ValidateRedirectURL(address); !errors.Is(err, ErrRedirectNotAllowed) {
				t.Fatalf("ValidateRedirectURL(%q) = %v, want ErrRedirectNotAllowed", address, err)
			}
		})
	}
}

func TestWithNoConfiguredOriginsEveryAddressIsRefused(t *testing.T) {
	// A store that configured nothing gets the provider's own result page, not
	// whatever a client asks for.
	if err := (Policy{}).ValidateRedirectURL("https://shop.example/done"); !errors.Is(err, ErrRedirectNotAllowed) {
		t.Fatalf("ValidateRedirectURL() = %v, want ErrRedirectNotAllowed", err)
	}
	if err := (Policy{}).ValidateRedirectURL(""); err != nil {
		t.Fatalf("an empty address must stay allowed, got %v", err)
	}
}

func TestConfiguredOriginsAreNormalizedAndMalformedOnesDropped(t *testing.T) {
	got := NormalizeRedirectOrigins([]string{"https://Shop.Example:443/", " https://shop.example ", "not a url", "ftp://files.example", "", "http://[::1]:8080"})
	want := []string{"https://shop.example", "http://[::1]:8080"}
	if len(got) != len(want) {
		t.Fatalf("NormalizeRedirectOrigins() = %v, want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("NormalizeRedirectOrigins() = %v, want %v", got, want)
		}
	}
}

func TestAPolicyBuiltByHandNeedNotNormalizeItsOrigins(t *testing.T) {
	// Only Bootstrap called NormalizeRedirectOrigins. Any other composition
	// root that listed the storefront the way it is usually written got a
	// policy that refused the storefront itself.
	policy := Policy{AllowedRedirectOrigins: []string{" https://Shop.Example:443/checkout "}}
	if err := policy.ValidateRedirectURL("https://shop.example/done"); err != nil {
		t.Fatalf("ValidateRedirectURL() = %v, want the storefront accepted", err)
	}
	if err := policy.ValidateRedirectURL("https://evil.example/done"); !errors.Is(err, ErrRedirectNotAllowed) {
		t.Fatalf("ValidateRedirectURL() = %v, want another site refused", err)
	}
}
