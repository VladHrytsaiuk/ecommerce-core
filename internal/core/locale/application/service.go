package application

import (
	"context"
	"fmt"
	"strings"

	localeDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/core/locale/domain"
)

type Service struct{ repo localeDomain.Repository }

func NewService(repo localeDomain.Repository) *Service { return &Service{repo: repo} }

// Synchronize makes configured locales available to normalized translation
// tables. It activates configured locales but intentionally does not delete or
// deactivate historic locales: existing content and order snapshots remain
// readable after a configuration change.
func (s *Service) Synchronize(ctx context.Context, supported []string, defaultLocale string) error {
	defaultLocale = normalize(defaultLocale)
	if defaultLocale == "" {
		return fmt.Errorf("default locale is required")
	}
	locales := make([]localeDomain.Locale, 0, len(supported))
	seen := make(map[string]struct{}, len(supported))
	for _, rawCode := range supported {
		code := normalize(rawCode)
		if code == "" {
			return fmt.Errorf("locale code is required")
		}
		if _, exists := seen[code]; exists {
			return fmt.Errorf("duplicate locale %q", code)
		}
		seen[code] = struct{}{}
		locales = append(locales, localeDomain.Locale{
			Code: code, Name: code, IsDefault: code == defaultLocale, IsActive: true,
		})
	}
	if _, exists := seen[defaultLocale]; !exists {
		return fmt.Errorf("default locale %q is not configured", defaultLocale)
	}
	return s.repo.Synchronize(ctx, locales)
}

func normalize(value string) string { return strings.ToLower(strings.TrimSpace(value)) }
