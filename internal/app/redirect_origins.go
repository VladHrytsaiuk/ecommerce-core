package app

import (
	"net"
	"net/url"
	"strings"

	checkoutDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/checkout/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/config"
)

// redirectOriginsFor is where a payment provider may return a buyer: the
// storefront's configured origins, less loopback ones in production.
//
// FRONTEND_URL defaults to http://localhost:3000, so a production store that
// never set it listed the buyer's own machine as a valid return address.
// Development keeps it, because that is exactly where its storefront runs.
func redirectOriginsFor(cfg *config.Config) []string {
	// A new slice, so CORSAllowOrigins is not appended to in place.
	origins := checkoutDomain.NormalizeRedirectOrigins(append([]string{cfg.FrontendURL}, cfg.CORSAllowOrigins...))
	if cfg.Env != "production" {
		return origins
	}
	kept := make([]string, 0, len(origins))
	for _, origin := range origins {
		if !isLoopbackOrigin(origin) {
			kept = append(kept, origin)
		}
	}
	return kept
}

func isLoopbackOrigin(origin string) bool {
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	host := parsed.Hostname()
	if strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
