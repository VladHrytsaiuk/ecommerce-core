package cartowner

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestSetSessionCookieUsesConfiguredSecurityPolicy(t *testing.T) {
	for _, secure := range []bool{false, true} {
		t.Run(map[bool]string{false: "development", true: "production"}[secure], func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			SetSessionCookie(context, uuid.New(), secure)
			cookie := recorder.Result().Cookies()[0]
			if cookie.Secure != secure || !cookie.HttpOnly {
				t.Fatalf("cookie security = (secure=%t, httpOnly=%t), want (%t, true)", cookie.Secure, cookie.HttpOnly, secure)
			}
		})
	}
}
