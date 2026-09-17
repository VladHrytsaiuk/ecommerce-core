package config

import (
	"strings"
	"testing"
	"time"
)

func TestValidateSignInCodeTTL(t *testing.T) {
	for name, testCase := range map[string]struct {
		ttl  time.Duration
		want bool
	}{
		"the default":       {10 * time.Minute, true},
		"the shortest":      {time.Minute, true},
		"the longest":       {30 * time.Minute, true},
		"under a minute":    {59 * time.Second, false},
		"over half an hour": {31 * time.Minute, false},
		"zero":              {0, false},
		"negative":          {-time.Minute, false},
	} {
		t.Run(name, func(t *testing.T) {
			err := validateSignInCodeTTL(testCase.ttl)
			if testCase.want && err != nil {
				t.Fatalf("validateSignInCodeTTL(%s) = %v", testCase.ttl, err)
			}
			if !testCase.want && (err == nil || !strings.Contains(err.Error(), "SIGN_IN_CODE_TTL")) {
				t.Fatalf("validateSignInCodeTTL(%s) = %v, want a SIGN_IN_CODE_TTL error", testCase.ttl, err)
			}
		})
	}
}
