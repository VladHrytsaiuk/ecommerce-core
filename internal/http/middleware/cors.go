package middleware

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// NewCORS creates an exact-origin CORS policy. Credentialed browser requests
// never use wildcard origins, methods, or request headers.
func NewCORS(origins []string) (gin.HandlerFunc, error) {
	allowed, err := validateOrigins(origins)
	if err != nil {
		return nil, err
	}
	return cors.New(cors.Config{
		AllowOrigins:     allowed,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "Accept-Language", "Idempotency-Key", "X-Request-ID"},
		ExposeHeaders:    []string{"X-Request-ID", "Retry-After"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}), nil
}

func validateOrigins(origins []string) ([]string, error) {
	allowed := make([]string, 0, len(origins))
	seen := make(map[string]struct{}, len(origins))
	for _, raw := range origins {
		origin := strings.TrimSpace(raw)
		if origin == "" {
			continue
		}
		parsed, err := url.Parse(origin)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Path != "" && parsed.Path != "/" {
			return nil, fmt.Errorf("invalid CORS origin %q", origin)
		}
		origin = strings.TrimSuffix(origin, "/")
		if _, ok := seen[origin]; ok {
			continue
		}
		seen[origin] = struct{}{}
		allowed = append(allowed, origin)
	}
	if len(allowed) == 0 {
		return nil, fmt.Errorf("at least one CORS origin is required")
	}
	return allowed, nil
}
