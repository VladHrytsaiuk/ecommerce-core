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
