package sanitize

import (
	"errors"
	"testing"
)

func TestErrorCodeNeverReturnsProviderMessageOrEmail(t *testing.T) {
	err := errors.New("smtp rejected buyer@example.com: message body secret")
	if code := ErrorCode(err); code != "invalid_recipient" {
		t.Fatalf("ErrorCode() = %q", code)
	}
}
