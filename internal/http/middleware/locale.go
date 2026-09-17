package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
)

const LanguageKey = "language"

type LocaleOptions struct {
	DefaultLocale    string
	FallbackLocale   string
	SupportedLocales []string
}

// NewLocaleMiddleware resolves a locale exclusively from validated store policy.
func NewLocaleMiddleware(options LocaleOptions) gin.HandlerFunc {
	fallbackLocale := normalizeLocale(options.FallbackLocale)
	supported := make(map[string]struct{}, len(options.SupportedLocales))
	for _, locale := range options.SupportedLocales {
		if locale = normalizeLocale(locale); locale != "" {
			supported[locale] = struct{}{}
		}
	}
	return func(c *gin.Context) {
		locale := normalizeLocale(c.Param("lang"))
		if _, ok := supported[locale]; !ok {
			locale = fallbackLocale
		}
		c.Set(LanguageKey, locale)
		c.Next()
	}
}

func GetLanguage(c *gin.Context) string {
	value, _ := c.Get(LanguageKey)
	locale, _ := value.(string)
	return locale
}

func normalizeLocale(value string) string { return strings.ToLower(strings.TrimSpace(value)) }
