package middleware

import (
	"net/http"
	"strings"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/security/token"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	authorizationHeaderKey  = "Authorization"
	authorizationTypeBearer = "bearer"
	authorizationPayloadKey = "user_id"
	authorizationRoleKey    = "role"
)

// AuthMiddleware створює gin-middleware для перевірки JWT токена.
func AuthMiddleware(tokenMaker token.Maker) gin.HandlerFunc {
	return func(c *gin.Context) {
		authorizationHeader := c.GetHeader(authorizationHeaderKey)

		if len(authorizationHeader) == 0 {
			logger.Log.Warn("missing authorization header")
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Authorization header is not provided"})
			return
		}

		fields := strings.Fields(authorizationHeader)
		if len(fields) < 2 {
			logger.Log.Warn("invalid authorization header format")
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid authorization header format. Expected 'Bearer {token}'"})
			return
		}

		authorizationType := strings.ToLower(fields[0])
		if authorizationType != authorizationTypeBearer {
			logger.Log.Warn("unsupported authorization type")
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unsupported authorization type"})
			return
		}

		accessToken := fields[1]
		claims, err := tokenMaker.VerifyToken(accessToken)
		if err != nil {
			logger.Log.Warnw("failed to verify access token", "error", err)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
			return
		}

		// Зберігаємо userID та roleID в контексті для подальшого використання у хендлерах
		c.Set(authorizationPayloadKey, claims.UserID)
		c.Set(authorizationRoleKey, claims.Role)
		c.Next()
	}
}

// AuthenticatedUserID returns the subject set by AuthMiddleware. Delivery
// handlers use this helper rather than depending on an internal Gin key.
func AuthenticatedUserID(c *gin.Context) (uuid.UUID, bool) {
	value, exists := c.Get(authorizationPayloadKey)
	userID, ok := value.(uuid.UUID)
	return userID, exists && ok && userID != uuid.Nil
}
