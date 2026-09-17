package config

import (
	"strings"
	"testing"
	"time"
)

func TestValidateOneTimeCodeTTL(t *testing.T) {
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
			err := validateOneTimeCodeTTL(testCase.ttl)
			if testCase.want && err != nil {
				t.Fatalf("validateOneTimeCodeTTL(%s) = %v", testCase.ttl, err)
			}
			if !testCase.want && (err == nil || !strings.Contains(err.Error(), "ONE_TIME_CODE_TTL")) {
				t.Fatalf("validateOneTimeCodeTTL(%s) = %v, want a ONE_TIME_CODE_TTL error", testCase.ttl, err)
			}
		})
	}
}

func TestAProductionStoreDoesNotTextCodesToTheLog(t *testing.T) {
	if err := validateSMSProvider("production", "mock", true); err == nil || !strings.Contains(err.Error(), "SMS_PROVIDER") {
		t.Fatalf("validateSMSProvider(production, mock, phone codes) = %v, want refused", err)
	}
	for _, allowed := range [][3]any{{"production", "vodafone", true}, {"development", "mock", true}, {"production", "mock", false}} {
		if err := validateSMSProvider(allowed[0].(string), allowed[1].(string), allowed[2].(bool)); err != nil {
			t.Fatalf("validateSMSProvider(%v) = %v", allowed, err)
		}
	}
}
