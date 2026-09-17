package http

import (
	"errors"
	std "net/http"

	"github.com/gin-gonic/gin"

	consent "github.com/VladHrytsaiuk/ecommerce-core/internal/consent/application"
	domain "github.com/VladHrytsaiuk/ecommerce-core/internal/consent/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/apiresponse"
	shared "github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware"
)

type grantConsentRequest struct {
	Type    string `json:"document_type"`
	Version string `json:"version"`
}

type privacyRequestPayload struct {
	Type string `json:"request_type"`
}

func RegisterV1Routes(v1 *gin.RouterGroup, s *consent.Service, auth gin.HandlerFunc, e *apiresponse.ErrorRenderer) {
	if v1 == nil || s == nil || e == nil {
		return
	}
	v1.GET("/legal/documents/active", listActiveDocuments(s, e))
	// Deliberately POST and deliberately unauthenticated. A guest has no
	// session, so the signed token is what speaks for them; and mail security
	// scanners follow links in messages, so a GET that withdraws consent would
	// unsubscribe customers whose provider simply checked the link was safe.
	// A store's page opens on a GET link and posts this.
	v1.POST("/consent/unsubscribe", unsubscribeFromMarketing(s, e))

	g := v1.Group("/customers/me/consents")
	g.Use(auth)
	g.GET("", listConsents(s, e))
	g.POST("", grantConsent(s, e))
	g.DELETE("/:type", withdrawConsent(s, e))

	p := v1.Group("/customers/me/privacy-requests")
	p.Use(auth)
	p.POST("", submitPrivacyRequest(s, e))
}

// listActiveDocuments godoc
// @Summary List the currently published legal documents
// @Description Public: a visitor must be able to read the terms before consenting to them.
// @Tags Consent v1
// @Produce json
// @Success 200 {object} apiresponse.SuccessResponse
// @Failure 500 {object} apiresponse.ProblemDetails
// @Router /api/v1/legal/documents/active [get]
func listActiveDocuments(s *consent.Service, e *apiresponse.ErrorRenderer) gin.HandlerFunc {
	return func(c *gin.Context) {
		v, err := s.ActiveDocuments(c)
		if err != nil {
			e.Abort(c, err)
			return
		}
		apiresponse.Success(c, std.StatusOK, v)
	}
}

type unsubscribeRequest struct {
	Token string `json:"token" binding:"required,max=512"`
}

// unsubscribeFromMarketing godoc
// @Summary Withdraw marketing consent using a signed link token
// @Description For a guest contact with no account. The token is issued to one address, expires, and is the only thing that authorises the withdrawal.
// @Tags Consent v1
// @Accept json
// @Produce json
// @Param payload body unsubscribeRequest true "Signed token from the email"
// @Success 204
// @Failure 400,422 {object} apiresponse.ProblemDetails
// @Router /api/v1/consent/unsubscribe [post]
func unsubscribeFromMarketing(s *consent.Service, e *apiresponse.ErrorRenderer) gin.HandlerFunc {
	return func(c *gin.Context) {
		var request unsubscribeRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			e.Abort(c, apiresponse.InvalidPayload(err))
			return
		}
		if err := s.UnsubscribeFromMarketing(c, request.Token); err != nil {
			abort(e, c, err)
			return
		}
		apiresponse.NoContent(c)
	}
}

// listConsents godoc
// @Summary Read the authenticated customer's consent history
// @Tags Consent v1
// @Produce json
// @Success 200 {object} apiresponse.SuccessResponse
// @Failure 401 {object} apiresponse.ProblemDetails
// @Router /api/v1/customers/me/consents [get]
func listConsents(s *consent.Service, e *apiresponse.ErrorRenderer) gin.HandlerFunc {
	return func(c *gin.Context) {
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
	}
}

// grantConsent godoc
// @Summary Record consent to a specific document version
// @Description The version is part of the record: consent is to the text that was published, not to the document in general.
// @Tags Consent v1
// @Accept json
// @Produce json
// @Param payload body grantConsentRequest true "Document type and version"
// @Success 204 "No Content"
// @Failure 400,401,422 {object} apiresponse.ProblemDetails
// @Router /api/v1/customers/me/consents [post]
func grantConsent(s *consent.Service, e *apiresponse.ErrorRenderer) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := shared.AuthenticatedUserID(c)
		if !ok {
			e.Abort(c, apiresponse.Unauthenticated(nil))
			return
		}
		var r grantConsentRequest
		if err := c.ShouldBindJSON(&r); err != nil {
			e.Abort(c, apiresponse.InvalidPayload(err))
			return
		}
		if err := s.Grant(c, id, r.Type, r.Version, c.ClientIP()); err != nil {
			abort(e, c, err)
			return
		}
		apiresponse.NoContent(c)
	}
}

// withdrawConsent godoc
// @Summary Withdraw a previously granted consent
// @Description Withdrawing the terms a customer is served under is refused; the account has to be closed instead.
// @Tags Consent v1
// @Produce json
// @Param type path string true "Document type"
// @Success 204 "No Content"
// @Failure 400,401,422 {object} apiresponse.ProblemDetails
// @Router /api/v1/customers/me/consents/{type} [delete]
func withdrawConsent(s *consent.Service, e *apiresponse.ErrorRenderer) gin.HandlerFunc {
	return func(c *gin.Context) {
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
	}
}

// submitPrivacyRequest godoc
// @Summary Submit a GDPR privacy request
// @Description Accepted for asynchronous handling; an administrator approves it before anything is exported or erased. Erasure returns 501 where the deployment has no erasure implementation configured.
// @Tags Consent v1
// @Accept json
// @Produce json
// @Param payload body privacyRequestPayload true "Request type"
// @Success 202 {object} apiresponse.SuccessResponse
// @Failure 400,401,422,501 {object} apiresponse.ProblemDetails
// @Router /api/v1/customers/me/privacy-requests [post]
func submitPrivacyRequest(s *consent.Service, e *apiresponse.ErrorRenderer) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := shared.AuthenticatedUserID(c)
		if !ok {
			e.Abort(c, apiresponse.Unauthenticated(nil))
			return
		}
		var r privacyRequestPayload
		if err := c.ShouldBindJSON(&r); err != nil {
			e.Abort(c, apiresponse.InvalidPayload(err))
			return
		}
		if err := s.PrivacyRequest(c, id, r.Type); err != nil {
			abort(e, c, err)
			return
		}
		apiresponse.Success(c, std.StatusAccepted, gin.H{"status": "pending"})
	}
}

func abort(e *apiresponse.ErrorRenderer, c *gin.Context, err error) {
	if errors.Is(err, domain.ErrErasureUnsupported) {
		// Not a validation failure: the request is well formed, and this store
		// simply cannot carry it out. Saying so is the point.
		e.Abort(c, apiresponse.NotImplemented(err, "This store cannot carry out erasure requests. Contact support for how your data is handled."))
		return
	}
	if errors.Is(err, domain.ErrExportUnsupported) {
		e.Abort(c, apiresponse.NotImplemented(err, "This store cannot produce a data export. Contact support for how your data is handled."))
		return
	}
	if errors.Is(err, domain.ErrInvalidUnsubscribeToken) {
		// One message for every failure — wrong signature, wrong shape,
		// expired. Distinguishing them would make the endpoint an oracle.
		e.Abort(c, apiresponse.ValidationFailed(err))
		return
	}
	if errors.Is(err, domain.ErrDocumentInactive) || errors.Is(err, domain.ErrInvalid) || errors.Is(err, domain.ErrTermsWithdrawalBlocked) {
		e.Abort(c, apiresponse.ValidationFailed(err))
		return
	}
	e.Abort(c, err)
}
