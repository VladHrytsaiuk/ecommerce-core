// Package cartowner resolves the authenticated customer or anonymous browser
// session that owns a clean Cart. It is shared by Cart and Checkout HTTP
// adapters so Checkout never accepts an owner identity from a request body.
package cartowner

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/cart/domain"
)

const SessionCookie = "cart_session"

// FromContext resolves an authenticated customer first. For anonymous buyers,
// it reads the opaque cart session cookie or creates one that the caller must
// persist with SetSessionCookie.
func FromContext(c *gin.Context) (domain.Owner, bool, error) {
	if raw, ok := c.Get("user_id"); ok {
		if id, ok := raw.(uuid.UUID); ok && id != uuid.Nil {
			return domain.Owner{CustomerID: &id}, false, nil
		}
		return domain.Owner{}, false, fmt.Errorf("authenticated user id is invalid")
	}
	if raw, err := c.Cookie(SessionCookie); err == nil {
		id, parseErr := uuid.Parse(raw)
		if parseErr != nil || id == uuid.Nil {
			return domain.Owner{}, false, fmt.Errorf("cart session is invalid")
		}
		return domain.Owner{SessionID: &id}, false, nil
	}
	id := uuid.New()
	return domain.Owner{SessionID: &id}, true, nil
}

func SetSessionCookie(c *gin.Context, sessionID uuid.UUID, secure bool) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     SessionCookie,
		Value:    sessionID.String(),
		Path:     "/",
		MaxAge:   60 * 60 * 24 * 30,
		Secure:   secure,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}
