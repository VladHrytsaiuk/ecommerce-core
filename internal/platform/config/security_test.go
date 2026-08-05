package config

import "testing"

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
