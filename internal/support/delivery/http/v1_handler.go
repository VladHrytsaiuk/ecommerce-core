package http

import (
	"errors"
	stdhttp "net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/apiresponse"
	shared "github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware"
	support "github.com/VladHrytsaiuk/ecommerce-core/internal/support/application"
	supportDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/support/domain"
)

const maxSupportRequestBytes int64 = 32 << 10

type createTicketRequest struct {
	Email   string `json:"email"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}
type messageRequest struct {
	Body string `json:"body"`
}

func RegisterV1Routes(v1 *gin.RouterGroup, service *support.Service, renderer *apiresponse.ErrorRenderer, optionalAuth gin.HandlerFunc, auth gin.HandlerFunc) {
	if v1 == nil || service == nil || renderer == nil {
		return
	}
	g := v1.Group("/support/tickets")
	if optionalAuth != nil {
		g.Use(optionalAuth)
	}
	g.POST("", func(c *gin.Context) {
		c.Request.Body = stdhttp.MaxBytesReader(c.Writer, c.Request.Body, maxSupportRequestBytes)
		var request createTicketRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			renderer.Abort(c, apiresponse.InvalidPayload(err))
			return
		}
		var customerID *uuid.UUID
		if id, ok := shared.AuthenticatedUserID(c); ok {
			customerID = &id
		}
		ticket, err := service.CreateTicket(c.Request.Context(), customerID, request.Email, request.Subject, request.Body, c.ClientIP())
		if err != nil {
			abort(renderer, c, err)
			return
		}
		apiresponse.Success(c, stdhttp.StatusCreated, gin.H{"id": ticket.ID, "status": ticket.Status, "created_at": ticket.CreatedAt})
	})
	// An authenticated subject is required for follow-up messages. This avoids
	// an IDOR on guest tickets until a signed, single-purpose guest token exists.
	messages := g.Group("/:id/messages")
	if auth != nil {
		messages.Use(auth)
	}
	messages.POST("", func(c *gin.Context) {
		c.Request.Body = stdhttp.MaxBytesReader(c.Writer, c.Request.Body, maxSupportRequestBytes)
		customerID, ok := shared.AuthenticatedUserID(c)
		if !ok {
			renderer.Abort(c, apiresponse.Unauthenticated(nil))
			return
		}
		ticketID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			renderer.Abort(c, apiresponse.InvalidPayload(err))
			return
		}
		var request messageRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			renderer.Abort(c, apiresponse.InvalidPayload(err))
			return
		}
		if err := service.AddCustomerMessage(c.Request.Context(), ticketID, customerID, request.Body, c.ClientIP(), ""); err != nil {
			abort(renderer, c, err)
			return
		}
		apiresponse.Success(c, stdhttp.StatusCreated, gin.H{"ticket_id": ticketID})
	})
}
func abort(renderer *apiresponse.ErrorRenderer, c *gin.Context, err error) {
	if errors.Is(err, supportDomain.ErrTicketNotFound) {
		renderer.Abort(c, apiresponse.NotFound(err, "Support ticket was not found."))
		return
	}
	if errors.Is(err, supportDomain.ErrSpam) {
		renderer.Abort(c, apiresponse.RateLimited(err))
		return
	}
	if errors.Is(err, supportDomain.ErrInvalidTicket) {
		renderer.Abort(c, apiresponse.ValidationFailed(err))
		return
	}
	if errors.Is(err, supportDomain.ErrMessageForbidden) {
		renderer.Abort(c, apiresponse.Forbidden(err))
		return
	}
	renderer.Abort(c, err)
}
