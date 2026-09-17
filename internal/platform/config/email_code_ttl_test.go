package config

import (
	"strings"
	"testing"
	"time"
)

func TestValidateEmailCodeTTL(t *testing.T) {
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
			err := validateEmailCodeTTL(testCase.ttl)
			if testCase.want && err != nil {
				t.Fatalf("validateEmailCodeTTL(%s) = %v", testCase.ttl, err)
			}
			if !testCase.want && (err == nil || !strings.Contains(err.Error(), "EMAIL_CODE_TTL")) {
				t.Fatalf("validateEmailCodeTTL(%s) = %v, want a EMAIL_CODE_TTL error", testCase.ttl, err)
			}
		})
	}
}
