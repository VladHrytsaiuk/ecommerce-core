package mock

import "testing"

func TestMaskEmail(t *testing.T) {
	if got := maskEmail("buyer@example.com"); got != "b***@example.com" {
		t.Fatalf("maskEmail() = %q", got)
	}
}
