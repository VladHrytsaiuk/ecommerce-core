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
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware"
)

const SessionCookie = "cart_session"

// FromContext resolves an authenticated customer first. For anonymous buyers,
// it reads the opaque cart session cookie or creates one that the caller must
// persist with SetSessionCookie.
func FromContext(c *gin.Context) (domain.Owner, bool, error) {
	// Resolve the subject through the middleware's own accessor. Reading the
	// Gin key directly meant renaming it there would not fail the build here:
	// this lookup would simply stop matching and every authenticated buyer
	// would silently fall through to the anonymous cookie branch below, taking
	// a guest cart instead of their own.
	if id, ok := middleware.AuthenticatedUserID(c); ok {
		return domain.Owner{CustomerID: &id}, false, nil
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

// GuestSessionID reads an existing anonymous session without creating a new
// one. Login flows use it to merge a pre-login wishlist into the user owner.
func GuestSessionID(c *gin.Context) (*uuid.UUID, error) {
	raw, err := c.Cookie(SessionCookie)
	if err != nil {
		return nil, nil
	}
	id, err := uuid.Parse(raw)
	if err != nil || id == uuid.Nil {
		return nil, fmt.Errorf("cart session is invalid")
	}
	return &id, nil
}
