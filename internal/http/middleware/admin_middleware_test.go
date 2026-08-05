package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/security/token"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestAdminMiddlewareAllowsOnlyStringAdminRoles(t *testing.T) {
	tests := []struct {
		name string
		role string
		want int
	}{
		{name: "admin", role: "admin", want: http.StatusNoContent},
		{name: "owner", role: "owner", want: http.StatusNoContent},
		{name: "customer", role: "customer", want: http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			maker := &MockTokenMaker{}
			maker.On("VerifyToken", "token").Return(&token.CustomClaims{UserID: uuid.New(), Role: tt.role}, nil)
			r := gin.New()
			r.GET("/admin", AuthMiddleware(maker), AdminMiddleware(), func(c *gin.Context) { c.Status(http.StatusNoContent) })
			request := httptest.NewRequest(http.MethodGet, "/admin", nil)
			request.Header.Set("Authorization", "Bearer token")
			response := httptest.NewRecorder()
			r.ServeHTTP(response, request)
			assert.Equal(t, tt.want, response.Code)
		})
	}
}
