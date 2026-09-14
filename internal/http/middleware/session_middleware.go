package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	GuestSessionCookie = "guest_session"
	GuestSessionKey    = "guest_session_id"
	CookieMaxAge       = 14 * 24 * 60 * 60 // 14 days, in seconds
)

// SessionMiddleware gives an anonymous visitor the stable identity their cart,
// wishlist and comparison list are keyed by, minting one when the cookie is
// absent.
//
// secure sets the cookie's Secure flag, which production requires. It also
// decides SameSite: a storefront on another origin needs SameSite=None, and a
// browser only accepts that together with Secure, so the two are chosen as a
// pair rather than independently.
func SessionMiddleware(secure bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID := c.GetHeader("X-Session-ID")
		if sessionID == "" {
			sessionID, _ = c.Cookie(GuestSessionCookie)
		}

		// Absent or blank: mint one.
		if sessionID == "" {
			sessionID = uuid.New().String()

			// SameSite=None is what lets a storefront on another origin send this
			// cookie at all, and no browser accepts it without Secure.
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

		// Echoed so a non-browser client can carry the session itself.
		c.Header("X-Session-ID", sessionID)

		// Publish it for the handlers below.
		c.Set(GuestSessionKey, sessionID)

		c.Next()
	}
}
