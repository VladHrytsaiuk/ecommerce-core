package middleware

import (
	"net/http"
	"strings"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/security/token"
	"github.com/gin-gonic/gin"
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

		// Тимчасовий дебаг для перевірки формату в Swagger
		logger.Log.Debugw("Received Authorization Header", "header", authorizationHeader)

		if len(authorizationHeader) == 0 {
			logger.Log.Warn("Authorization header is not provided")
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Authorization header is not provided"})
			return
		}

		fields := strings.Fields(authorizationHeader)
		if len(fields) < 2 {
			logger.Log.Warnw("Invalid authorization header format", "header", authorizationHeader)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid authorization header format. Expected 'Bearer {token}'"})
			return
		}

		authorizationType := strings.ToLower(fields[0])
		if authorizationType != authorizationTypeBearer {
			logger.Log.Warnw("Unsupported authorization type", "type", authorizationType)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unsupported authorization type"})
			return
		}

		accessToken := fields[1]
		claims, err := tokenMaker.VerifyToken(accessToken)
		if err != nil {
			tokenPrefix := accessToken
			if len(tokenPrefix) > 10 {
				tokenPrefix = tokenPrefix[:10]
			}
			logger.Log.Warnw("Failed to verify token", "error", err, "token_prefix", tokenPrefix+"...")
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
			return
		}

		// Зберігаємо userID та roleID в контексті для подальшого використання у хендлерах
		c.Set(authorizationPayloadKey, claims.UserID)
		c.Set(authorizationRoleKey, claims.Role)
		c.Next()
	}
}
