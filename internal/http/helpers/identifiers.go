package helpers

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware"
)

// GetIdentifiers витягує user_id з JWT контексту або session_id з куки/контексту.
// Повертає (userID, sessionID).
//
// Пріоритет:
//  1. Авторизований юзер (JWT → user_id в контексті)
//  2. Анонімний юзер (session_id з контексту, встановлений SessionMiddleware)
//  3. Fallback: session_id з куки напряму
func GetIdentifiers(c *gin.Context) (*uuid.UUID, *string) {
	// 1. Пріоритет — авторизований юзер
	if userIDRaw, exists := c.Get("user_id"); exists {
		var userID uuid.UUID
		switch v := userIDRaw.(type) {
		case string:
			userID, _ = uuid.Parse(v)
		case uuid.UUID:
			userID = v
		}
		if userID != uuid.Nil {
			return &userID, nil
		}
	}

	// 2. Якщо не авторизований — беремо session_id з контексту (встановлений SessionMiddleware)
	if sessionIDRaw, exists := c.Get(middleware.GuestSessionKey); exists {
		if sID, ok := sessionIDRaw.(string); ok && sID != "" {
			return nil, &sID
		}
	}

	// 3. Fallback: спробувати прочитати з заголовка або куки напряму
	if sID := c.GetHeader("X-Session-ID"); sID != "" {
		return nil, &sID
	}
	if sID, err := c.Cookie(middleware.GuestSessionCookie); err == nil && sID != "" {
		return nil, &sID
	}

	return nil, nil
}

// GetGuestSessionID витягує session_id (анонімної сесії) з контексту або з куки напряму,
// навіть якщо користувач вже авторизований (має user_id).
func GetGuestSessionID(c *gin.Context) *string {
	if sessionIDRaw, exists := c.Get(middleware.GuestSessionKey); exists {
		if sID, ok := sessionIDRaw.(string); ok && sID != "" {
			return &sID
		}
	}

	if sID := c.GetHeader("X-Session-ID"); sID != "" {
		return &sID
	}
	if sID, err := c.Cookie(middleware.GuestSessionCookie); err == nil && sID != "" {
		return &sID
	}

	return nil
}
