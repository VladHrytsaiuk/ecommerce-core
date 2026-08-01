package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	LanguageKey = "language"
	DefaultLang = "uk" // Legacy fallback for handlers used outside a locale route.
	LangParam   = "lang"
)

// LocaleOptions is a delivery-layer view of the store locale policy.
type LocaleOptions struct {
	DefaultLocale    string
	FallbackLocale   string
	SupportedLocales []string
}

// NewLocaleMiddleware resolves a route locale using store configuration.
// The temporary ua -> uk alias preserves compatibility with existing clients.
func NewLocaleMiddleware(options LocaleOptions) gin.HandlerFunc {
	defaultLocale := normalizeLocale(options.DefaultLocale)
	fallbackLocale := normalizeLocale(options.FallbackLocale)
	if defaultLocale == "" {
		defaultLocale = DefaultLang
	}
	if fallbackLocale == "" {
		fallbackLocale = defaultLocale
	}

	supported := make(map[string]struct{}, len(options.SupportedLocales))
	for _, locale := range options.SupportedLocales {
		if locale = normalizeLocale(locale); locale != "" {
			supported[locale] = struct{}{}
		}
	}
	if len(supported) == 0 {
		supported[defaultLocale] = struct{}{}
	}

	return func(c *gin.Context) {
		locale := normalizeLocale(c.Param(LangParam))
		if locale == "ua" {
			locale = "uk"
		}
		if _, ok := supported[locale]; !ok {
			locale = fallbackLocale
		}

		c.Set(LanguageKey, locale)
		c.Next()
	}
}

// LocaleMiddleware preserves the previous default behavior for callers that
// have not yet been migrated to a store-provided locale policy.
func LocaleMiddleware() gin.HandlerFunc {
	return NewLocaleMiddleware(LocaleOptions{
		DefaultLocale:    DefaultLang,
		FallbackLocale:   DefaultLang,
		SupportedLocales: []string{"uk", "en"},
	})
}

// GetLanguage safely returns the resolved request locale.
func GetLanguage(c *gin.Context) string {
	if locale, exists := c.Get(LanguageKey); exists {
		if value, ok := locale.(string); ok && value != "" {
			return value
		}
	}
	return DefaultLang
}

func normalizeLocale(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
