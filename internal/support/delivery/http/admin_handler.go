package http

import (
	"errors"
	stdhttp "net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	adminDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/apiresponse"
	shared "github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware"
	support "github.com/VladHrytsaiuk/ecommerce-core/internal/support/application"
	supportDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/support/domain"
)

const (
	PermissionSupportRead  = "support:read"
	PermissionSupportWrite = "support:write"
)

type adminHandler struct {
	service *support.Service
	errors  *apiresponse.ErrorRenderer
}

func RegisterV1AdminRoutes(group *gin.RouterGroup, authorizer adminDomain.Authorizer, service *support.Service, renderer *apiresponse.ErrorRenderer) {
	if group == nil || authorizer == nil || service == nil || renderer == nil {
		return
	}
	h := &adminHandler{service, renderer}
	g := group.Group("/support/tickets")
	g.GET("", shared.RequirePermissionV1(authorizer, PermissionSupportRead, renderer), h.list)
	g.GET("/:id", shared.RequirePermissionV1(authorizer, PermissionSupportRead, renderer), h.get)
	g.POST("/:id/messages", shared.RequirePermissionV1(authorizer, PermissionSupportWrite, renderer), h.reply)
	g.PATCH("/:id/status", shared.RequirePermissionV1(authorizer, PermissionSupportWrite, renderer), h.transition)
}
func (h *adminHandler) list(c *gin.Context) {
	page, limit := pageLimit(c)
	tickets, total, err := h.service.ListTickets(c.Request.Context(), c.Query("status"), page, limit)
	if err != nil {
		h.abort(c, err)
		return
	}
	h.data(c, stdhttp.StatusOK, gin.H{"tickets": tickets, "total": total, "page": page, "limit": limit})
}
func (h *adminHandler) get(c *gin.Context) {
	id, ok := h.id(c)
	if !ok {
		return
	}
	ticket, messages, err := h.service.GetTicket(c.Request.Context(), id)
	if err != nil {
		h.abort(c, err)
		return
	}
	h.data(c, stdhttp.StatusOK, gin.H{"ticket": ticket, "messages": messages})
}
func (h *adminHandler) reply(c *gin.Context) {
	id, ok := h.id(c)
	if !ok {
		return
	}
	agent, ok := shared.AuthenticatedUserID(c)
	if !ok {
		h.errors.Abort(c, apiresponse.Unauthenticated(nil))
		return
	}
	var r messageRequest
	if err := c.ShouldBindJSON(&r); err != nil {
		h.errors.Abort(c, apiresponse.InvalidPayload(err))
		return
	}
	ticket, err := h.service.ReplyAsAgent(c.Request.Context(), id, agent, r.Body)
	if err != nil {
		h.abort(c, err)
		return
	}
	h.data(c, stdhttp.StatusCreated, ticket)
}
func (h *adminHandler) transition(c *gin.Context) {
	id, ok := h.id(c)
	if !ok {
		return
	}
	agent, ok := shared.AuthenticatedUserID(c)
	if !ok {
		h.errors.Abort(c, apiresponse.Unauthenticated(nil))
		return
	}
	var r struct {
		Status string `json:"status"`
	}
	if err := c.ShouldBindJSON(&r); err != nil {
		h.errors.Abort(c, apiresponse.InvalidPayload(err))
		return
	}
	ticket, err := h.service.ChangeStatus(c.Request.Context(), id, agent, strings.TrimSpace(r.Status))
	if err != nil {
		h.abort(c, err)
		return
	}
	h.data(c, stdhttp.StatusOK, ticket)
}
func (h *adminHandler) id(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil || id == uuid.Nil {
		h.errors.Abort(c, apiresponse.InvalidPayload(err))
		return uuid.Nil, false
	}
	return id, true
}
func (h *adminHandler) data(c *gin.Context, status int, data any) {
	apiresponse.Success(c, status, data)
}
func (h *adminHandler) abort(c *gin.Context, err error) {
	if errors.Is(err, supportDomain.ErrTicketNotFound) {
		h.errors.Abort(c, apiresponse.NotFound(err, "Support ticket was not found."))
		return
	}
	h.errors.Abort(c, apiresponse.ValidationFailed(err))
}
func pageLimit(c *gin.Context) (int, int) {
	page, limit := 1, 20
	if v, e := strconv.Atoi(c.DefaultQuery("page", "1")); e == nil && v > 0 && v <= 1000 {
		page = v
	}
	if v, e := strconv.Atoi(c.DefaultQuery("limit", "20")); e == nil && v > 0 && v <= 100 {
		limit = v
	}
	return page, limit
}
