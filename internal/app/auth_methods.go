package app

import (
	"fmt"
	"sort"
	"strings"
)

// AuthMethod is one way a customer can sign in, selected through AUTH_METHODS.
// Any combination may be enabled together; a store picks the ones it offers.
type AuthMethod string

const (
	// AuthMethodPassword is registration and sign-in with an address and a
	// password.
	AuthMethodPassword AuthMethod = "password"
	// AuthMethodGoogle is sign-in with Google.
	AuthMethodGoogle AuthMethod = "google"
	// AuthMethodEmailCode is registration and sign-in with a one-time code sent
	// to an email address. It needs the notifications module to send the code.
	AuthMethodEmailCode AuthMethod = "email_code"
	// AuthMethodPhoneCode is registration and sign-in with a one-time code sent
	// by text to a phone number. It needs SMS_PROVIDER.
	AuthMethodPhoneCode AuthMethod = "phone_code"
)

// allAuthMethods is every name AUTH_METHODS accepts.
var allAuthMethods = []AuthMethod{AuthMethodPassword, AuthMethodGoogle, AuthMethodEmailCode, AuthMethodPhoneCode}

// EmailCodesAvailable reports whether this store sends one-time codes by
// email: to verify an address after registration, to reset a password and, with
// email_code, to sign in. They go out through the notifications module.
func (c StoreConfig) EmailCodesAvailable() bool {
	return c.Modules().Has(ModuleNotifications)
}

// AuthMethodSet is the normalized set of enabled sign-in methods.
type AuthMethodSet map[AuthMethod]struct{}

// Has reports whether a sign-in method is enabled.
func (s AuthMethodSet) Has(method AuthMethod) bool {
	_, enabled := s[method]
	return enabled
}

// resolveAuthMethods turns AUTH_METHODS into the set of enabled methods.
//
// Left unset it reproduces exactly what this core did before the setting
// existed: password sign-in always, and Google whenever its credentials are
// configured. Set explicitly, it is held to the credentials: Google listed
// without them fails, and so do Google credentials that no listed method uses —
// a setting present but ignored is a misconfiguration, not a default.
func resolveAuthMethods(raw []string, googleConfigured bool) (AuthMethodSet, error) {
	names := normalizeAll(raw)
	if len(names) == 0 {
		methods := AuthMethodSet{AuthMethodPassword: {}}
		if googleConfigured {
			methods[AuthMethodGoogle] = struct{}{}
		}
		return methods, nil
	}
	methods := make(AuthMethodSet, len(names))
	unknown := make([]string, 0)
	for _, name := range names {
		method := AuthMethod(name)
		if !method.known() {
			unknown = append(unknown, name)
			continue
		}
		methods[method] = struct{}{}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return nil, fmt.Errorf("AUTH_METHODS contains unknown method(s): %s; valid methods are %s",
			strings.Join(unknown, ", "), strings.Join(authMethodNames(), ", "))
	}
	if methods.Has(AuthMethodGoogle) && !googleConfigured {
		return nil, fmt.Errorf("AUTH_METHODS includes google, which requires GOOGLE_CLIENT_ID, GOOGLE_CLIENT_SECRET and GOOGLE_REDIRECT_URI")
	}
	if !methods.Has(AuthMethodGoogle) && googleConfigured {
		return nil, fmt.Errorf("GOOGLE_CLIENT_ID is configured but AUTH_METHODS does not include google; add google to AUTH_METHODS or remove the Google settings")
	}
	return methods, nil
}

func (m AuthMethod) known() bool {
	for _, method := range allAuthMethods {
		if method == m {
			return true
		}
	}
	return false
}

func authMethodNames() []string {
	names := make([]string, 0, len(allAuthMethods))
	for _, method := range allAuthMethods {
		names = append(names, string(method))
	}
	sort.Strings(names)
	return names
}
