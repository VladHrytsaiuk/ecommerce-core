package application

import (
	"context"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/ratelimit"
	support "github.com/VladHrytsaiuk/ecommerce-core/internal/support/domain"
	"strings"
	"time"
)

type SpamProtector struct{ limiter ratelimit.Service }

func NewSpamProtector(l ratelimit.Service) *SpamProtector { return &SpamProtector{l} }
func (p *SpamProtector) Check(ctx context.Context, ip, email, body string) error {
	if len([]rune(strings.TrimSpace(body))) == 0 || len([]rune(body)) > 2000 {
		return support.ErrSpam
	}
	for _, k := range []string{"support:ip:" + ip, "support:email:" + strings.ToLower(strings.TrimSpace(email))} {
		d, e := p.limiter.Allow(ctx, k, 3, time.Hour)
		if e != nil || !d.Allowed {
			return support.ErrSpam
		}
	}
	return nil
}

var _ support.SpamProtector = (*SpamProtector)(nil)
