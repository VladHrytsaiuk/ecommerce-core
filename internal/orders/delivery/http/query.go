package http

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

func queryInt(c *gin.Context, name string, fallback, min, max int) (int, bool) {
	raw := strings.TrimSpace(c.Query(name))
	if raw == "" {
		return fallback, true
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < min || value > max {
		return 0, false
	}
	return value, true
}
