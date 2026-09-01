// Package http exposes only Returns application ports; ownership is always
// taken from JWT, never from customer identifiers in request bodies.
package http

import (
	"errors"
	"io"
	stdhttp "net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	adminDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/apiresponse"
	shared "github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware"
	returnsApp "github.com/VladHrytsaiuk/ecommerce-core/internal/returns/application"
	returns "github.com/VladHrytsaiuk/ecommerce-core/internal/returns/domain"
)

const PermissionReturnsWrite = "returns:write"

type customerHandler struct {
	service *returnsApp.ReturnService
	errors  *apiresponse.ErrorRenderer
}

type adminHandler struct{ customerHandler }

func RegisterV1CustomerRoutes(v1 *gin.RouterGroup, service *returnsApp.ReturnService, auth gin.HandlerFunc, renderer *apiresponse.ErrorRenderer) {
	if v1 == nil || service == nil || auth == nil || renderer == nil {
		return
	}
	h := &customerHandler{service: service, errors: renderer}
	group := v1.Group("/customers/me/returns")
	group.Use(auth)
	group.POST("", h.Create)
}

func RegisterV1AdminRoutes(group *gin.RouterGroup, authorizer adminDomain.Authorizer, service *returnsApp.ReturnService, renderer *apiresponse.ErrorRenderer) {
	if group == nil || authorizer == nil || service == nil || renderer == nil {
		return
	}
	h := &adminHandler{customerHandler{service: service, errors: renderer}}
	group.POST("/returns/:id/approve", shared.RequirePermissionV1(authorizer, PermissionReturnsWrite, renderer), h.Approve)
	group.POST("/returns/:id/receive", shared.RequirePermissionV1(authorizer, PermissionReturnsWrite, renderer), h.Receive)
}

type createRequest struct {
	OrderID    uuid.UUID          `json:"order_id" binding:"required"`
	RefundMode returns.RefundMode `json:"refund_mode"`
	Items      []itemRequest      `json:"items" binding:"required,min=1,max=50"`
}

type itemRequest struct {
	VariantID uuid.UUID             `json:"variant_id" binding:"required"`
	Quantity  int                   `json:"quantity" binding:"required,min=1,max=10000"`
	Condition returns.ItemCondition `json:"condition" binding:"required"`
	Reason    string                `json:"reason" binding:"max=2000"`
}

type reasonRequest struct {
	Reason string `json:"reason" binding:"max=2000"`
}

// Create godoc
// @Summary Create a return request for the current customer (v1)
// @Tags Returns v1
// @Accept json
// @Produce json
// @Param payload body createRequest true "Return request"
// @Success 201 {object} apiresponse.SuccessResponse
// @Failure 400,401,422 {object} apiresponse.ProblemDetails
// @Router /api/v1/customers/me/returns [post]
func (h *customerHandler) Create(c *gin.Context) {
	customerID, ok := shared.AuthenticatedUserID(c)
	if !ok {
		h.errors.Abort(c, apiresponse.Unauthenticated(nil))
		return
	}
	var request createRequest
	if err := c.ShouldBindJSON(&request); err != nil || request.OrderID == uuid.Nil || len(request.Items) == 0 {
		h.errors.Abort(c, apiresponse.InvalidPayload(err))
		return
	}
	items := make([]returns.ReturnItem, 0, len(request.Items))
	for _, item := range request.Items {
		items = append(items, returns.ReturnItem{VariantID: item.VariantID, Quantity: item.Quantity, Condition: item.Condition, Reason: strings.TrimSpace(item.Reason)})
	}
	created, err := h.service.CreateReturnRequest(c.Request.Context(), returnsApp.CreateCommand{OrderID: request.OrderID, CustomerID: customerID, RefundMode: request.RefundMode, Items: items})
	if err != nil {
		h.abort(c, err)
		return
	}
	apiresponse.Success(c, stdhttp.StatusCreated, response(*created))
}

// Approve godoc
// @Summary Approve a return request (v1 admin)
// @Tags Admin v1
// @Accept json
// @Produce json
// @Param id path string true "Return request UUID"
// @Param payload body reasonRequest false "Approval reason"
// @Success 200 {object} apiresponse.SuccessResponse
// @Failure 400,401,403,404,422 {object} apiresponse.ProblemDetails
// @Router /api/v1/admin/returns/{id}/approve [post]
func (h *adminHandler) Approve(c *gin.Context) { h.transition(c, true) }

// Receive godoc
// @Summary Receive a return and schedule controlled settlement (v1 admin)
// @Tags Admin v1
// @Accept json
// @Produce json
// @Param id path string true "Return request UUID"
// @Param payload body reasonRequest false "Receipt reason"
// @Success 202 {object} apiresponse.SuccessResponse
// @Failure 400,401,403,404,422 {object} apiresponse.ProblemDetails
// @Router /api/v1/admin/returns/{id}/receive [post]
func (h *adminHandler) Receive(c *gin.Context) { h.transition(c, false) }

func (h *adminHandler) transition(c *gin.Context, approve bool) {
	adminID, ok := shared.AuthenticatedUserID(c)
	if !ok {
		h.errors.Abort(c, apiresponse.Unauthenticated(nil))
		return
	}
	returnID, err := uuid.Parse(c.Param("id"))
	if err != nil || returnID == uuid.Nil {
		h.errors.Abort(c, apiresponse.InvalidPayload(err, apiresponse.InvalidParam{Field: "id", Code: "invalid_uuid"}))
		return
	}
	var request reasonRequest
	if err := c.ShouldBindJSON(&request); err != nil && !errors.Is(err, io.EOF) {
		h.errors.Abort(c, apiresponse.InvalidPayload(err))
		return
	}
	var value *returns.ReturnRequest
	if approve {
		value, err = h.service.ApproveReturn(c.Request.Context(), returnID, adminID, strings.TrimSpace(request.Reason))
	} else {
		value, err = h.service.ReceiveReturn(c.Request.Context(), returnID, adminID, strings.TrimSpace(request.Reason))
	}
	if err != nil {
		h.abort(c, err)
		return
	}
	status := stdhttp.StatusOK
	if !approve {
		status = stdhttp.StatusAccepted
	}
	apiresponse.Success(c, status, response(*value))
}

func (h *customerHandler) abort(c *gin.Context, err error) {
	switch {
	case errors.Is(err, returns.ErrReturnRequestNotFound):
		h.errors.Abort(c, apiresponse.NotFound(err, "The requested return was not found."))
	case errors.Is(err, returns.ErrReturnOrderMismatch), errors.Is(err, returns.ErrReturnCustomerMismatch):
		h.errors.Abort(c, apiresponse.Forbidden(err))
	case errors.Is(err, returns.ErrReturnOrderNotDelivered), errors.Is(err, returns.ErrReturnWindowExpired), errors.Is(err, returns.ErrReturnEligibilityDateMiss), errors.Is(err, returns.ErrInvalidReturnRequest), errors.Is(err, returns.ErrInvalidReturnItem), errors.Is(err, returns.ErrInvalidReturnTransition), errors.Is(err, returnsApp.ErrUnsupportedRefundMode):
		h.errors.Abort(c, apiresponse.ValidationFailed(err))
	default:
		h.errors.Abort(c, err)
	}
}

func response(request returns.ReturnRequest) gin.H {
	items := make([]gin.H, 0, len(request.Items))
	for _, item := range request.Items {
		items = append(items, gin.H{"id": item.ID, "variant_id": item.VariantID, "quantity": item.Quantity, "condition": item.Condition, "reason": item.Reason})
	}
	return gin.H{"id": request.ID, "order_id": request.OrderID, "status": request.Status, "refund_mode": request.RefundMode, "items": items, "created_at": request.CreatedAt, "updated_at": request.UpdatedAt}
}
