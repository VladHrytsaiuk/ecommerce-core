//go:build legacy && ignore
// +build legacy,ignore

package http

import (
	"errors"
	"net/http"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/order/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/pagination"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type AdminOrderHandler struct {
	service domain.AdminOrderService
	l       logger.Logger
}

func NewAdminOrderHandler(s domain.AdminOrderService, l logger.Logger) *AdminOrderHandler {
	return &AdminOrderHandler{service: s, l: l}
}

// @Summary      List orders (Admin)
// @Description  Get a paginated and filtered list of orders
// @Tags         Admin Orders
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        status_id    query     int       false "Filter by Status ID"
// @Param        search       query     string    false "Search by number, email, phone, name"
// @Param        date_from    query     string    false "From date (YYYY-MM-DD)"
// @Param        date_to      query     string    false "To date (YYYY-MM-DD)"
// @Param        page         query     int       false "Page number"
// @Param        limit        query     int       false "Items per page"
// @Param        sort_by      query     string    false "Sort field (created_at, total_price)"
// @Param        order        query     string    false "Sort direction (asc, desc)"
// @Success      200          {object}  map[string]interface{}
// @Failure      400          {object}  ErrorResponse
// @Failure      401          {object}  ErrorResponse
// @Failure      403          {object}  ErrorResponse
// @Router       /admin/orders [get]
func (h *AdminOrderHandler) ListOrders(c *gin.Context) {
	var filters domain.AdminOrderFilters
	if err := c.ShouldBindQuery(&filters); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid filters", Message: err.Error()})
		return
	}

	orders, meta, err := h.service.ListOrders(c.Request.Context(), filters)
	if err != nil {
		h.l.Errorw("failed to list admin orders", "error", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Internal Server Error", Message: "Failed to list orders"})
		return
	}

	c.JSON(http.StatusOK, pagination.PagedResponse{
		Metadata: meta,
		Data:     MapToAdminList(orders),
	})
}

// @Summary      Get order details (Admin)
// @Description  Get full details of an order including history
// @Tags         Admin Orders
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      string  true  "Order ID (UUID)"
// @Success      200  {object}  AdminOrderDetailsResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Router       /admin/orders/{id} [get]
func (h *AdminOrderHandler) GetOrderDetails(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid ID", Message: "Valid UUID required"})
		return
	}

	order, history, payment, err := h.service.GetOrderByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrOrderNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "Not Found", Message: "Order not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Internal Server Error", Message: "Failed to fetch order"})
		return
	}

	c.JSON(http.StatusOK, MapToAdminDetails(order, history, payment))
}

// @Summary      Get order statuses
// @Description  Get list of all available order statuses
// @Tags         Admin Orders
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Success      200  {array}   OrderStatusDTO
// @Router       /admin/orders/statuses [get]
func (h *AdminOrderHandler) GetStatuses(c *gin.Context) {
	statuses, err := h.service.GetOrderStatuses(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Internal Server Error", Message: "Failed to fetch statuses"})
		return
	}

	dtos := make([]OrderStatusDTO, len(statuses))
	for i, s := range statuses {
		dtos[i] = OrderStatusDTO{
			ID:   s.ID,
			Code: s.Code,
			Name: s.Name["uk"],
		}
	}
	c.JSON(http.StatusOK, dtos)
}

// @Summary      Update admin comment
// @Description  Update the internal admin comment for an order
// @Tags         Admin Orders
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      string                     true  "Order ID (UUID)"
// @Param        body body      UpdateAdminCommentRequest  true  "Comment"
// @Success      200  {object}  map[string]string
// @Router       /admin/orders/{id}/comment [put]
func (h *AdminOrderHandler) UpdateComment(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid ID", Message: "Valid UUID required"})
		return
	}

	var req UpdateAdminCommentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid Body", Message: err.Error()})
		return
	}

	if err := h.service.UpdateAdminComment(c.Request.Context(), id, req.Comment); err != nil {
		if errors.Is(err, domain.ErrOrderNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "Not Found", Message: "Order not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Internal Server Error", Message: "Failed to update comment"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Comment updated successfully"})
}

// @Summary      Confirm order (Admin)
// @Description  Confirm order and create TTN
// @Tags         Admin Orders
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      string  true  "Order ID (UUID)"
// @Success      200  {object}  ConfirmOrderResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      409  {object}  ErrorResponse
// @Failure      422  {object}  ErrorResponse
// @Router       /admin/orders/{id}/confirm [post]
func (h *AdminOrderHandler) ConfirmOrder(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid ID", Message: "Valid UUID required"})
		return
	}

	adminID, err := uuid.Parse(c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "Unauthorized", Message: "Invalid admin ID"})
		return
	}

	order, err := h.service.ConfirmOrder(c.Request.Context(), id, adminID)
	if err != nil {
		h.handleAdminError(c, err)
		return
	}

	c.JSON(http.StatusOK, ConfirmOrderResponse{
		OrderNumber: order.OrderNumber,
		TTNNumber:   order.TTNNumber,
		Status:      "shipped",
		Message:     "Замовлення підтверджено, ТТН успішно створено",
	})
}

// @Summary      Cancel order (Admin)
// @Description  Cancel an order
// @Tags         Admin Orders
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      string  true  "Order ID (UUID)"
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  ErrorResponse
// @Failure      409  {object}  ErrorResponse
// @Router       /admin/orders/{id}/cancel [post]
func (h *AdminOrderHandler) CancelOrder(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid ID", Message: "Valid UUID required"})
		return
	}

	adminID, err := uuid.Parse(c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "Unauthorized", Message: "Invalid admin ID"})
		return
	}

	err = h.service.CancelOrder(c.Request.Context(), id, adminID)
	if err != nil {
		h.handleAdminError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Order cancelled successfully", "status": "cancelled"})
}

func (h *AdminOrderHandler) handleAdminError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrOrderNotFound):
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "Not Found", Message: "Order not found"})
	case errors.Is(err, domain.ErrOrderNotProcessing):
		c.JSON(http.StatusUnprocessableEntity, ErrorResponse{Error: "Unprocessable Entity", Message: "Order must be in processing status"})
	case errors.Is(err, domain.ErrOrderAlreadyCancelled):
		c.JSON(http.StatusConflict, ErrorResponse{Error: "Conflict", Message: "Order is already cancelled"})
	case errors.Is(err, domain.ErrCancelBlockedAfterTTN):
		c.JSON(http.StatusConflict, ErrorResponse{Error: "Conflict", Message: "Cannot cancel order after TTN is created"})
	default:
		h.l.Errorw("unhandled admin error", "error", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Internal Server Error", Message: "An unexpected error occurred"})
	}
}
