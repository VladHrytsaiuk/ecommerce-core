// Package apiresponse defines versioned HTTP response contracts. It belongs to
// the transport layer and must never be imported by domain or application code.
package apiresponse

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
)

// SuccessResponse is the standard v1 envelope for a single resource.
type SuccessResponse struct {
	Data      any    `json:"data"`
	RequestID string `json:"request_id"`
}

// PageMetadata has stable, client-oriented names independent of repository
// pagination internals.
type PageMetadata struct {
	Page        int   `json:"page"`
	Limit       int   `json:"limit"`
	Total       int64 `json:"total"`
	TotalPages  int   `json:"total_pages"`
	HasNext     bool  `json:"has_next"`
	HasPrevious bool  `json:"has_previous"`
}

// PaginatedResponse is the standard v1 envelope for collection resources.
type PaginatedResponse struct {
	Data      any          `json:"data"`
	Meta      PageMetadata `json:"meta"`
	RequestID string       `json:"request_id"`
}

func Success(c *gin.Context, status int, data any) {
	c.JSON(status, SuccessResponse{Data: data, RequestID: requestID(c)})
}

func Paginated(c *gin.Context, status int, data any, meta PageMetadata) {
	c.JSON(status, PaginatedResponse{Data: data, Meta: meta, RequestID: requestID(c)})
}

func requestID(c *gin.Context) string {
	if id := logger.RequestID(c.Request.Context()); id != "" {
		return id
	}
	if id := c.Writer.Header().Get("X-Request-ID"); id != "" {
		return id
	}
	// ObservabilityMiddleware normally supplies this value before a handler is
	// reached. The fallback preserves the v1 response invariant in focused
	// handler tests and in deliberately minimal embeddings.
	id := uuid.NewString()
	c.Header("X-Request-ID", id)
	return id
}

// NoContent is retained here to make the response package the sole v1 place
// where an endpoint intentionally has no response body.
func NoContent(c *gin.Context) { c.Status(http.StatusNoContent) }
