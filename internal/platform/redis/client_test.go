package redis

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestConnectDoesNotExposeCredentialsForMalformedURL(t *testing.T) {
	const password = "highly-sensitive-redis-password"
	_, err := Connect(context.Background(), "redis://:"+password+"@127.0.0.1:6379/%zz")
	if !errors.Is(err, ErrInvalidURL) {
		t.Fatalf("Connect() error = %v, want ErrInvalidURL", err)
	}
	if strings.Contains(err.Error(), password) {
		t.Fatalf("Connect() leaked password in error: %q", err)
	}
}
