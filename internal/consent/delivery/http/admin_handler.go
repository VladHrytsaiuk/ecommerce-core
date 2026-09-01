package http

import (
	admin "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/domain"
	consent "github.com/VladHrytsaiuk/ecommerce-core/internal/consent/application"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/apiresponse"
	shared "github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"net/http"
	"strconv"
)

const (
	PermissionLegalWrite   = "legal:write"
	PermissionPrivacyWrite = "privacy:write"
)

func RegisterV1AdminRoutes(g *gin.RouterGroup, a admin.Authorizer, s *consent.Service, e *apiresponse.ErrorRenderer) {
	if g == nil || a == nil || s == nil {
		return
	}
	g.POST("/legal/documents", shared.RequirePermissionV1(a, PermissionLegalWrite, e), func(c *gin.Context) {
		var r struct {
			Type       string `json:"type"`
			Version    string `json:"version"`
			ContentURL string `json:"content_url"`
		}
		if c.ShouldBindJSON(&r) != nil {
			e.Abort(c, apiresponse.InvalidPayload(nil))
			return
		}
		x, err := s.CreateDocument(c, r.Type, r.Version, r.ContentURL)
		if err != nil {
			e.Abort(c, apiresponse.ValidationFailed(err))
			return
		}
		apiresponse.Success(c, http.StatusCreated, x)
	})
	g.POST("/legal/documents/:id/publish", shared.RequirePermissionV1(a, PermissionLegalWrite, e), func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			e.Abort(c, apiresponse.InvalidPayload(err))
			return
		}
		x, err := s.PublishDocument(c, id)
		if err != nil {
			e.Abort(c, err)
			return
		}
		apiresponse.Success(c, http.StatusOK, x)
	})
	g.GET("/privacy-requests", shared.RequirePermissionV1(a, PermissionPrivacyWrite, e), func(c *gin.Context) {
		p, l := 1, 20
		if x, _ := strconv.Atoi(c.Query("page")); x > 0 {
			p = x
		}
		if x, _ := strconv.Atoi(c.Query("limit")); x > 0 && x <= 100 {
			l = x
		}
		x, n, err := s.ListPrivacyRequests(c, p, l)
		if err != nil {
			e.Abort(c, err)
			return
		}
		apiresponse.Success(c, http.StatusOK, gin.H{"requests": x, "total": n})
	})
	g.POST("/privacy-requests/:id/approve", shared.RequirePermissionV1(a, PermissionPrivacyWrite, e), func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			e.Abort(c, apiresponse.InvalidPayload(err))
			return
		}
		x, err := s.ApprovePrivacyRequest(c, id)
		if err != nil {
			e.Abort(c, err)
			return
		}
		apiresponse.Success(c, http.StatusAccepted, x)
	})
}
