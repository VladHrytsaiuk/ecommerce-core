package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	LanguageKey = "language"
	DefaultLang = "uk"
	LangParam   = "lang"
)

var SupportedLanguages = map[string]bool{
	"uk": true,
	"en": true,
}

// LocaleMiddleware витягує код мови з параметра URL (наприклад, :lang),
// перевіряє підтримку і зберігає в контексті запиту.
func LocaleMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		lang := c.Param(LangParam)
		lang = strings.ToLower(strings.TrimSpace(lang))

		// Перетворення "ua" (яке часто використовують на фронті) у стандарт "uk"
		if lang == "ua" {
			lang = "uk"
		}

		if !SupportedLanguages[lang] {
			// Відповідно до вимог fallback на українську мову
			lang = DefaultLang
		}

		c.Set(LanguageKey, lang)
		c.Next()
	}
}

// GetLanguage - допоміжна функція для безпечного отримання мови в хендлерах
func GetLanguage(c *gin.Context) string {
	if lang, exists := c.Get(LanguageKey); exists {
		return lang.(string)
	}
	return DefaultLang
}
