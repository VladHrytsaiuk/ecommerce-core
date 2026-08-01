package middleware

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// TimeoutMiddleware додає таймаут до контексту запиту.
// Це дозволяє GORM та іншим сервісам перервати виконання при перевищенні ліміту.
func TimeoutMiddleware(timeout time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Створюємо контекст з таймаутом на основі існуючого контексту запиту
		ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
		defer cancel()

		// Оновлюємо контекст запиту
		c.Request = c.Request.WithContext(ctx)

		c.Next()

		// Якщо після виконання хендлерів виявлено таймаут, і відповідь ще не відправлена
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			if !c.Writer.Written() {
				c.AbortWithStatusJSON(http.StatusGatewayTimeout, gin.H{
					"error":   "Gateway Timeout",
					"message": "The request took too long to process",
				})
			}
		}
	}
}
