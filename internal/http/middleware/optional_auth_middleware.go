package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/security/token"
)

// OptionalAuthMiddleware працює як AuthMiddleware, але не блокує запит
// якщо токен відсутній. Якщо токен присутній і валідний — встановлює
// user_id та role_id в контекст. Якщо токен відсутній — просто пропускає далі.
// Корисно для ендпоінтів, які доступні і анонімним, і авторизованим юзерам (наприклад, вішліст).
func OptionalAuthMiddleware(tokenMaker token.Maker) gin.HandlerFunc {
	return func(c *gin.Context) {
		authorizationHeader := c.GetHeader(authorizationHeaderKey)

		// Якщо заголовка немає — просто пропускаємо
		if len(authorizationHeader) == 0 {
			c.Next()
			return
		}

		fields := strings.Fields(authorizationHeader)
		if len(fields) < 2 {
			// Невалідний формат — пропускаємо як anonymous
			c.Next()
			return
		}

		authorizationType := strings.ToLower(fields[0])
		if authorizationType != authorizationTypeBearer {
			c.Next()
			return
		}

		accessToken := fields[1]
		claims, err := tokenMaker.VerifyToken(accessToken)
		if err != nil {
			logger.Log.Debugw("OptionalAuth: token verification failed, proceeding as anonymous", "error", err)
			c.Next()
			return
		}

		// Токен валідний — зберігаємо user_id та role_id в контексті
		c.Set(authorizationPayloadKey, claims.UserID)
		c.Set(authorizationRoleKey, claims.RoleID)
		c.Next()
	}
}
