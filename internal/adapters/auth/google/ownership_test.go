package google

import "testing"

// Google's email_verified says an address was confirmed, not that the account
// still holds it. Only an address in a domain Google runs, or in the Workspace
// domain the token names, proves current ownership.
func TestOnlyAnAddressGoogleRunsProvesCurrentOwnership(t *testing.T) {
	for name, testCase := range map[string]struct {
		email        string
		verified     bool
		hostedDomain string
		want         bool
	}{
		"a gmail address":                  {"buyer@gmail.com", true, "", true},
		"a googlemail address":             {"buyer@googlemail.com", true, "", true},
		"gmail in another case":            {"Buyer@GMAIL.com", true, "", true},
		"a workspace address":              {"buyer@store.example", true, "store.example", true},
		"a workspace domain in any case":   {"buyer@store.example", true, "Store.Example", true},
		"an outlook address":               {"buyer@outlook.com", true, "", false},
		"a custom domain without hd":       {"buyer@store.example", true, "", false},
		"hd for another domain":            {"buyer@store.example", true, "other.example", false},
		"an unconfirmed gmail address":     {"buyer@gmail.com", false, "", false},
		"an unconfirmed custom address":    {"buyer@store.example", false, "store.example", false},
		"something that is not an address": {"buyer", true, "", false},
	} {
		t.Run(name, func(t *testing.T) {
			if got := ownsAddress(testCase.email, testCase.verified, testCase.hostedDomain); got != testCase.want {
				t.Fatalf("ownsAddress(%q, %v, %q) = %v, want %v", testCase.email, testCase.verified, testCase.hostedDomain, got, testCase.want)
			}
		})
	}
}
