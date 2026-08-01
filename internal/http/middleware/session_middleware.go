package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	GuestSessionCookie = "guest_session"
	GuestSessionKey    = "guest_session_id"
	CookieMaxAge       = 14 * 24 * 60 * 60 // 14 днів у секундах
)

// SessionMiddleware перевіряє наявність куки сесії для анонімного кошика/вішліста.
// Якщо куки немає — генерує нову і встановлює її.
// Параметр secure вказує, чи встановлювати прапорець Secure для cookie (true для HTTPS у production).
// Для cross-origin (фронт на іншому домені) Secure=true → SameSite=None, інакше SameSite=Lax.
func SessionMiddleware(secure bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID := c.GetHeader("X-Session-ID")
		if sessionID == "" {
			sessionID, _ = c.Cookie(GuestSessionCookie)
		}

		// Якщо куки/заголовка немає або вона порожня — генеруємо нову
		if sessionID == "" {
			sessionID = uuid.New().String()

			// SameSite=None потрібен для cross-origin запитів (фронт на Vercel, бекенд на іншому домені)
			// SameSite=None вимагає Secure=true
			sameSite := http.SameSiteLaxMode
			if secure {
				sameSite = http.SameSiteNoneMode
			}

			http.SetCookie(c.Writer, &http.Cookie{
				Name:     GuestSessionCookie,
				Value:    sessionID,
				MaxAge:   CookieMaxAge,
				Path:     "/",
				Secure:   secure,
				HttpOnly: true,
				SameSite: sameSite,
			})
		}

		// Додаємо заголовок X-Session-ID до відповіді
		c.Header("X-Session-ID", sessionID)

		// Зберігаємо в контексті
		c.Set(GuestSessionKey, sessionID)

		c.Next()
	}
}
