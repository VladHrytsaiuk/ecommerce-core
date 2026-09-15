package config

import (
	"strings"
	"testing"
	"time"
)

func TestValidateRefreshTokenDuration(t *testing.T) {
	access := 15 * time.Minute
	for name, testCase := range map[string]struct {
		refresh time.Duration
		want    string
	}{
		"a week":                        {7 * 24 * time.Hour, ""},
		"equal to the access token":     {access, ""},
		"shorter than the access token": {5 * time.Minute, "at least ACCESS_TOKEN_DURATION"},
		"zero":                          {0, "positive"},
		"negative":                      {-time.Hour, "positive"},
	} {
		t.Run(name, func(t *testing.T) {
			err := validateRefreshTokenDuration(testCase.refresh, access)
			if testCase.want == "" && err != nil {
				t.Fatalf("validateRefreshTokenDuration(%s) = %v", testCase.refresh, err)
			}
			if testCase.want != "" && (err == nil || !strings.Contains(err.Error(), testCase.want)) {
				t.Fatalf("validateRefreshTokenDuration(%s) = %v, want it to mention %q", testCase.refresh, err, testCase.want)
			}
		})
	}
}
