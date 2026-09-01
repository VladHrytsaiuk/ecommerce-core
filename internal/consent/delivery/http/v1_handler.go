package http

import (
	"errors"
	consent "github.com/VladHrytsaiuk/ecommerce-core/internal/consent/application"
	domain "github.com/VladHrytsaiuk/ecommerce-core/internal/consent/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/apiresponse"
	shared "github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	std "net/http"
)

func RegisterV1Routes(v1 *gin.RouterGroup, s *consent.Service, auth gin.HandlerFunc, e *apiresponse.ErrorRenderer) {
	if v1 == nil || s == nil || e == nil {
		return
	}
	v1.GET("/legal/documents/active", func(c *gin.Context) {
		v, err := s.ActiveDocuments(c)
		if err != nil {
			e.Abort(c, err)
			return
		}
		apiresponse.Success(c, std.StatusOK, v)
	})
	g := v1.Group("/customers/me/consents")
	g.Use(auth)
	g.GET("", func(c *gin.Context) {
		id, ok := shared.AuthenticatedUserID(c)
		if !ok {
			e.Abort(c, apiresponse.Unauthenticated(nil))
			return
		}
		v, err := s.Consents(c, id)
		if err != nil {
			e.Abort(c, err)
			return
		}
		apiresponse.Success(c, std.StatusOK, v)
	})
	g.POST("", func(c *gin.Context) {
		id, ok := shared.AuthenticatedUserID(c)
		if !ok {
			e.Abort(c, apiresponse.Unauthenticated(nil))
			return
		}
		var r struct {
			Type    string `json:"document_type"`
			Version string `json:"version"`
		}
		if err := c.ShouldBindJSON(&r); err != nil {
			e.Abort(c, apiresponse.InvalidPayload(err))
			return
		}
		if err := s.Grant(c, id, r.Type, r.Version, c.ClientIP()); err != nil {
			abort(e, c, err)
			return
		}
		apiresponse.NoContent(c)
	})
	g.DELETE("/:type", func(c *gin.Context) {
		id, ok := shared.AuthenticatedUserID(c)
		if !ok {
			e.Abort(c, apiresponse.Unauthenticated(nil))
			return
		}
		if err := s.Withdraw(c, id, c.Param("type")); err != nil {
			abort(e, c, err)
			return
		}
		apiresponse.NoContent(c)
	})
	p := v1.Group("/customers/me/privacy-requests")
	p.Use(auth)
	p.POST("", func(c *gin.Context) {
		id, ok := shared.AuthenticatedUserID(c)
		if !ok {
			e.Abort(c, apiresponse.Unauthenticated(nil))
			return
		}
		var r struct {
			Type string `json:"request_type"`
		}
		if err := c.ShouldBindJSON(&r); err != nil {
			e.Abort(c, apiresponse.InvalidPayload(err))
			return
		}
		if err := s.PrivacyRequest(c, id, r.Type); err != nil {
			abort(e, c, err)
			return
		}
		apiresponse.Success(c, std.StatusAccepted, gin.H{"status": "pending"})
	})
}
func abort(e *apiresponse.ErrorRenderer, c *gin.Context, err error) {
	if errors.Is(err, domain.ErrDocumentInactive) || errors.Is(err, domain.ErrInvalid) || errors.Is(err, domain.ErrTermsWithdrawalBlocked) {
		e.Abort(c, apiresponse.ValidationFailed(err))
		return
	}
	e.Abort(c, err)
}

var _ = uuid.Nil
