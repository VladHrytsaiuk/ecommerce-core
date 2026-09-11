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
	g.POST("", createTicket(service, renderer))
	// An authenticated subject is required for follow-up messages. This avoids
	// an IDOR on guest tickets until a signed, single-purpose guest token exists.
	messages := g.Group("/:id/messages")
	if auth != nil {
		messages.Use(auth)
	}
	messages.POST("", addCustomerMessage(service, renderer))
}

// createTicket godoc
// @Summary Open a support ticket
// @Description Public intake, protected by a distributed per-IP and per-email quota. A guest supplies an email; for an authenticated customer it is resolved server-side.
// @Tags Support v1
// @Accept json
// @Produce json
// @Param payload body createTicketRequest true "Ticket"
// @Success 201 {object} apiresponse.SuccessResponse
// @Failure 400,422,429 {object} apiresponse.ProblemDetails
// @Router /api/v1/support/tickets [post]
func createTicket(service *support.Service, renderer *apiresponse.ErrorRenderer) gin.HandlerFunc {
	return func(c *gin.Context) {
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
	}
}

// addCustomerMessage godoc
// @Summary Reply to your own support ticket
// @Description Requires authentication: ticket ownership is checked against the authenticated subject, so a guest ticket cannot be read or extended by guessing its ID.
// @Tags Support v1
// @Accept json
// @Produce json
// @Param id path string true "Ticket UUID"
// @Param payload body messageRequest true "Message"
// @Success 201 {object} apiresponse.SuccessResponse
// @Failure 400,401,403,404,422 {object} apiresponse.ProblemDetails
// @Router /api/v1/support/tickets/{id}/messages [post]
func addCustomerMessage(service *support.Service, renderer *apiresponse.ErrorRenderer) gin.HandlerFunc {
	return func(c *gin.Context) {
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
	}
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
