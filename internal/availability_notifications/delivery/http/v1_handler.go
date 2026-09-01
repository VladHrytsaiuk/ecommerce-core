package http

import (
	"fmt"
	availability "github.com/VladHrytsaiuk/ecommerce-core/internal/availability_notifications/application"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/apiresponse"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"net/http"
	"strings"
)

type request struct {
	Email string `json:"email" binding:"omitempty,max=320"`
}

func RegisterV1Routes(v1 *gin.RouterGroup, s *availability.Service, renderer *apiresponse.ErrorRenderer, optionalAuth gin.HandlerFunc) {
	if v1 == nil || s == nil {
		return
	}
	g := v1.Group("/catalog/variants/:id/subscribe")
	if optionalAuth != nil {
		g.Use(optionalAuth)
	}
	g.POST("", func(c *gin.Context) {
		id, e := uuid.Parse(c.Param("id"))
		var in request
		if e != nil || c.ShouldBindJSON(&in) != nil {
			renderer.Abort(c, apiresponse.InvalidPayload(e))
			return
		}
		var customerID uuid.UUID
		if value, ok := middleware.AuthenticatedUserID(c); ok {
			customerID = value
		}
		if strings.TrimSpace(in.Email) == "" && customerID == uuid.Nil {
			renderer.Abort(c, apiresponse.ValidationFailed(availabilityDomainError()))
			return
		}
		out, created, e := s.Subscribe(c.Request.Context(), id, in.Email, customerID)
		if e != nil {
			renderer.Abort(c, apiresponse.ValidationFailed(e))
			return
		}
		status := http.StatusCreated
		if !created {
			status = http.StatusOK
		}
		apiresponse.Success(c, status, gin.H{"id": out.ID, "variant_id": out.VariantID, "status": out.Status})
	})
}
func availabilityDomainError() error { return fmt.Errorf("email is required") }
