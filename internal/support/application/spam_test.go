package application

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/ratelimit"
	support "github.com/VladHrytsaiuk/ecommerce-core/internal/support/domain"
)

func TestSpamProtectorRejectsFourthTicketFromSameIP(t *testing.T) {
	p := NewSpamProtector(ratelimit.NewLocalService())
	for i := 0; i < 3; i++ {
		if err := p.Check(context.Background(), "192.0.2.10", "buyer@example.test", "Need help"); err != nil {
			t.Fatalf("attempt %d: %v", i+1, err)
		}
	}
	if err := p.Check(context.Background(), "192.0.2.10", "other@example.test", "Need help"); !errors.Is(err, support.ErrSpam) {
		t.Fatalf("fourth IP request error = %v, want ErrSpam", err)
	}
}

func TestSpamProtectorRejectsOversizedBody(t *testing.T) {
	p := NewSpamProtector(ratelimit.NewLocalService())
	if err := p.Check(context.Background(), "192.0.2.11", "buyer@example.test", strings.Repeat("a", 2001)); !errors.Is(err, support.ErrSpam) {
		t.Fatalf("error = %v, want ErrSpam", err)
	}
}
