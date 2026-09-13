package config

import "testing"

func TestParseTrustedProxies(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []string
		bad  bool
	}{
		{name: "defaults to loopback", want: []string{"127.0.0.1"}},
		{name: "accepts IPs and CIDRs", raw: "127.0.0.1, 10.0.0.0/8,2001:db8::/32", want: []string{"127.0.0.1", "10.0.0.0/8", "2001:db8::/32"}},
		{name: "rejects all", raw: "all", bad: true},
		{name: "rejects empty entry", raw: "127.0.0.1,", bad: true},
		{name: "rejects arbitrary hostname", raw: "proxy.internal", bad: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseTrustedProxies(test.raw)
			if (err != nil) != test.bad {
				t.Fatalf("parseTrustedProxies() error = %v, want error %t", err, test.bad)
			}
			if !test.bad && !equalStrings(got, test.want) {
				t.Fatalf("parseTrustedProxies() = %v, want %v", got, test.want)
			}
		})
	}
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestValidateStartupSecurity(t *testing.T) {
	tests := []struct {
		name, environment, secret, origins string
		wantErr                            bool
	}{
		{name: "rejects empty secret", environment: "development", wantErr: true},
		{name: "rejects legacy default secret", environment: "development", secret: insecureDefaultJWTSecret, wantErr: true},
		{name: "rejects production without origins", environment: "production", secret: "a-secure-secret-with-at-least-thirty-two-characters", wantErr: true},
		{name: "allows production with explicit origins", environment: "production", secret: "a-secure-secret-with-at-least-thirty-two-characters", origins: "https://store.example", wantErr: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateStartupSecurity(test.environment, test.secret, test.origins)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateStartupSecurity() error = %v, wantErr %t", err, test.wantErr)
			}
		})
	}
}

func TestValidateNotificationProvider(t *testing.T) {
	// The mock sender returns a successful receipt, so a production store on
	// the default reports every order confirmation as sent while none leaves
	// the building. Nothing downstream can notice: the job is marked sent.
	for name, scenario := range map[string]struct {
		env, provider string
		enabled       bool
		wantRefusal   bool
	}{
		"mock in production with notifications on": {"production", "mock", true, true},
		"mock spelled loudly":                      {"production", "  MOCK ", true, true},
		"mock in production, notifications off":    {"production", "mock", false, false},
		"mock in development":                      {"development", "mock", true, false},
		"smtp in production":                       {"production", "smtp", true, false},
		"ses in production":                        {"production", "ses", true, false},
	} {
		t.Run(name, func(t *testing.T) {
			err := validateNotificationProvider(scenario.env, scenario.provider, scenario.enabled)
			if scenario.wantRefusal && err == nil {
				t.Fatal("startup accepted a production store whose mail goes nowhere")
			}
			if !scenario.wantRefusal && err != nil {
				t.Fatalf("startup refused a valid configuration: %v", err)
			}
		})
	}
}

// ENABLED_MODULES is read once and canonicalised at that point, because every
// consumer downstream compares module names case-insensitively and this file
// did not. A store writing "Notifications" got the module enabled and both of
// the guards that validate it silently skipped.

func TestModuleNamesAreCanonicalisedWhenRead(t *testing.T) {
	for name, raw := range map[string][]string{
		"already canonical":  {"notifications"},
		"capitalised":        {"Notifications"},
		"shouted":            {"NOTIFICATIONS"},
		"padded":             {"  notifications  "},
		"padded and shouted": {" Notifications "},
	} {
		t.Run(name, func(t *testing.T) {
			if !containsModule(NormalizeModules(raw), "notifications") {
				t.Fatalf("%v did not register as the notifications module; its guards would not run", raw)
			}
		})
	}
}

func TestCanonicalisationDropsEmptyEntriesAndKeepsTheRest(t *testing.T) {
	// A trailing comma in the environment variable is an empty entry, not a
	// module, and must not become one.
	got := NormalizeModules([]string{"Notifications", "  ", "Abandoned_Cart", ""})

	if len(got) != 2 || got[0] != "notifications" || got[1] != "abandoned_cart" {
		t.Fatalf("NormalizeModules() = %q", got)
	}
}

func TestAnUnrelatedModuleIsStillNotEnabled(t *testing.T) {
	// The point is canonicalisation, not matching everything.
	if containsModule(NormalizeModules([]string{"Notifications"}), "abandoned_cart") {
		t.Fatal("containsModule matched a module that is not in the list")
	}
}
