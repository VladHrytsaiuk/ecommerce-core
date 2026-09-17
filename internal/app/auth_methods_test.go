package app

import (
	"strings"
	"testing"
)

func TestUnsetAuthMethodsKeepWhatTheCoreAlwaysDid(t *testing.T) {
	// Deployments that predate AUTH_METHODS must not notice it.
	withoutGoogle, err := resolveAuthMethods(nil, false)
	if err != nil || len(withoutGoogle) != 1 || !withoutGoogle.Has(AuthMethodPassword) {
		t.Fatalf("resolveAuthMethods(unset, no google) = (%v, %v), want password only", withoutGoogle, err)
	}
	withGoogle, err := resolveAuthMethods(nil, true)
	if err != nil || len(withGoogle) != 2 || !withGoogle.Has(AuthMethodPassword) || !withGoogle.Has(AuthMethodGoogle) {
		t.Fatalf("resolveAuthMethods(unset, google) = (%v, %v), want password and google", withGoogle, err)
	}
}

func TestAnyCombinationOfMethodsCanBeEnabled(t *testing.T) {
	methods, err := resolveAuthMethods([]string{" Google "}, true)
	if err != nil || len(methods) != 1 || !methods.Has(AuthMethodGoogle) || methods.Has(AuthMethodPassword) {
		t.Fatalf("resolveAuthMethods(google) = (%v, %v), want google alone, without password", methods, err)
	}
}

func TestAuthMethodsThatCannotWorkFailTheBoot(t *testing.T) {
	for name, testCase := range map[string]struct {
		raw              []string
		googleConfigured bool
		want             string
	}{
		"a typo":                          {[]string{"password", "gogle"}, false, "unknown method(s): gogle"},
		"google without credentials":      {[]string{"google"}, false, "requires GOOGLE_CLIENT_ID"},
		"credentials google does not use": {[]string{"password"}, true, "does not include google"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := resolveAuthMethods(testCase.raw, testCase.googleConfigured)
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("resolveAuthMethods(%v, %v) = %v, want an error mentioning %q", testCase.raw, testCase.googleConfigured, err, testCase.want)
			}
		})
	}
}

func TestAStoreNobodyCanSignInToDoesNotBoot(t *testing.T) {
	// Bootstrap validates a StoreConfig it did not build, so the rule holds there
	// too and not only where AUTH_METHODS is read.
	storeConfig, err := NewStoreConfig(validConfig())
	if err != nil {
		t.Fatal(err)
	}
	storeConfig.AuthMethods = nil
	if err := storeConfig.Validate(); err == nil || !strings.Contains(err.Error(), "sign-in method") {
		t.Fatalf("Validate() = %v, want a store with no sign-in method refused", err)
	}
	storeConfig.AuthMethods = AuthMethodSet{AuthMethodGoogle: {}}
	if err := storeConfig.Validate(); err == nil || !strings.Contains(err.Error(), "configured together") {
		t.Fatalf("Validate() = %v, want google without credentials refused", err)
	}
}
