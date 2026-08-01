package middleware

import (
	"bytes"
	"context"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/audit/domain"
)

var (
	// sensitiveRegex шукає поля типу password, token, secret у JSON та замінює їх значення
	sensitiveRegex = regexp.MustCompile(`(?i)"(password|token|secret|cvv|card|refresh_token)":\s*"[^"]*"`)
)

// AuditMiddleware перехоплює дії адміністратора та записує їх у базу даних.
func AuditMiddleware(auditSvc domain.AuditService) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. Початковий час запиту
		start := time.Now()

		// 2. Перехоплюємо тіло запиту (Body) для методів, що змінюють дані
		var bodyBytes []byte
		method := c.Request.Method
		isMutating := method == "POST" || method == "PUT" || method == "PATCH" || method == "DELETE"
		contentType := c.GetHeader("Content-Type")
		isMultipart := strings.Contains(contentType, "multipart/form-data")

		if isMutating && !isMultipart && c.Request.Body != nil {
			// Читаємо максимум 1MB для логу аудиту
			limitReader := io.LimitReader(c.Request.Body, 1024*1024)
			bodyBytes, _ = io.ReadAll(limitReader)

			// ВАЖЛИВО: Склеюємо прочитані байти та залишок потоку за допомогою MultiReader,
			// щоб хендлери отримали весь запит повністю, навіть якщо він більше 1МБ.
			c.Request.Body = io.NopCloser(io.MultiReader(bytes.NewBuffer(bodyBytes), c.Request.Body))
		}

		// 3. Виконуємо запит далі
		c.Next()

		// 4. Після виконання збираємо дані для аудиту

		// Отримуємо UserID з контексту
		userIDStr, exists := c.Get("user_id")
		var userID uuid.UUID
		if exists {
			if id, ok := userIDStr.(string); ok {
				userID, _ = uuid.Parse(id)
			} else if id, ok := userIDStr.(uuid.UUID); ok {
				userID = id
			}
		}

		// Обробка Payload: маскування чутливих даних
		payload := redactPayload(string(bodyBytes))
		if len(payload) > 4096 {
			payload = payload[:4096] + "... [truncated]"
		}

		logEntry := &domain.AuditLog{
			ID:         uuid.New(),
			UserID:     userID,
			Method:     method,
			Path:       c.Request.URL.Path,
			IP:         c.ClientIP(),
			UserAgent:  c.Request.UserAgent(),
			StatusCode: c.Writer.Status(),
			Payload:    payload,
			Duration:   time.Since(start).Milliseconds(),
			CreatedAt:  time.Now(),
		}

		// 5. Зберігаємо в базу даних асинхронно
		go func(log *domain.AuditLog) {
			// Даємо базі даних максимум 5 секунд на запис, щоб уникнути витоку горутин
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			_ = auditSvc.Log(ctx, log)
		}(logEntry)
	}
}

// redactPayload замінює значення чутливих полів на [REDACTED]
func redactPayload(payload string) string {
	if payload == "" {
		return ""
	}
	return sensitiveRegex.ReplaceAllString(payload, `"$1": "[REDACTED]"`)
}
