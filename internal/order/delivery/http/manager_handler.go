//go:build legacy
// +build legacy

package http

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/order/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/gin-gonic/gin"
)

// ManagerHandler обробляє HTTP запити для менеджерського доступу до замовлень
type ManagerHandler struct {
	service domain.ManagerService
	l       logger.Logger
}

// NewManagerHandler створює новий інстанс хендлера
func NewManagerHandler(s domain.ManagerService, l logger.Logger) *ManagerHandler {
	return &ManagerHandler{service: s, l: l}
}

// @Summary      Get manager order
// @Description  Get order details for manager using a token
// @Tags         Manager
// @Accept       json
// @Produce      json
// @Param        orderNumber  path      int     true  "Order Number"
// @Param        token        query     string  true  "Manager Token"
// @Success      200          {object}  ManagerOrderResponse
// @Failure      400          {object}  ErrorResponse
// @Failure      401          {object}  ErrorResponse
// @Failure      404          {object}  ErrorResponse
// @Router       /manager/orders/{orderNumber} [get]
// GetManagerOrder повертає замовлення для менеджера за token
func (h *ManagerHandler) GetManagerOrder(c *gin.Context) {
	orderNumber, token, ok := h.extractParams(c)
	if !ok {
		return
	}

	order, err := h.service.GetOrderByToken(c.Request.Context(), orderNumber, token)
	if err != nil {
		h.handleManagerError(c, err)
		return
	}

	c.JSON(http.StatusOK, h.mapToManagerResponse(order))
}

// @Summary      Confirm order
// @Description  Confirm order and create TTN using manager token
// @Tags         Manager
// @Accept       json
// @Produce      json
// @Param        orderNumber  path      int     true  "Order Number"
// @Param        token        query     string  true  "Manager Token"
// @Success      200          {object}  ConfirmOrderResponse
// @Failure      400          {object}  ErrorResponse
// @Failure      401          {object}  ErrorResponse
// @Failure      404          {object}  ErrorResponse
// @Failure      422          {object}  ErrorResponse
// @Router       /manager/orders/{orderNumber}/confirm [post]
// ConfirmOrder підтверджує замовлення
func (h *ManagerHandler) ConfirmOrder(c *gin.Context) {
	orderNumber, token, ok := h.extractParams(c)
	if !ok {
		return
	}

	order, err := h.service.ConfirmOrder(c.Request.Context(), orderNumber, token)
	if err != nil {
		h.handleManagerError(c, err)
		return
	}

	c.JSON(http.StatusOK, ConfirmOrderResponse{
		OrderNumber: order.OrderNumber,
		TTNNumber:   order.TTNNumber,
		Status:      "shipped",
		Message:     "Замовлення підтверджено, ТТН успішно створено",
	})
}

// @Summary      Cancel manager order
// @Description  Cancel an order using manager token
// @Tags         Manager
// @Accept       json
// @Produce      json
// @Param        orderNumber  path      int     true  "Order Number"
// @Param        token        query     string  true  "Manager Token"
// @Success      200          {object}  map[string]interface{}
// @Failure      400          {object}  ErrorResponse
// @Failure      401          {object}  ErrorResponse
// @Failure      404          {object}  ErrorResponse
// @Failure      409          {object}  ErrorResponse
// @Router       /manager/orders/{orderNumber}/cancel [post]
// CancelOrder скасовує замовлення
func (h *ManagerHandler) CancelOrder(c *gin.Context) {
	orderNumber, token, ok := h.extractParams(c)
	if !ok {
		return
	}

	err := h.service.CancelOrder(c.Request.Context(), orderNumber, token)
	if err != nil {
		h.handleManagerError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Замовлення скасовано",
		"status":  "cancelled",
	})
}

// extractParams витягує orderNumber та token з URL та query params
func (h *ManagerHandler) extractParams(c *gin.Context) (int64, string, bool) {
	orderNumberStr := c.Param("orderNumber")
	orderNumber, err := strconv.ParseInt(orderNumberStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Bad Request",
			Message: "Invalid order number format",
		})
		return 0, "", false
	}

	token := c.Query("token")
	if token == "" {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "Unauthorized",
			Message: "Manager token is required",
		})
		return 0, "", false
	}

	return orderNumber, token, true
}

// mapToManagerResponse маппить доменну модель у менеджерський DTO
func (h *ManagerHandler) mapToManagerResponse(o *domain.Order) ManagerOrderResponse {
	resp := ManagerOrderResponse{
		ID:          o.ID,
		OrderNumber: o.OrderNumber,
		Status: OrderStatusDTO{
			ID:   o.Status.ID,
			Code: o.Status.Code,
		},
		FirstName:  o.FirstName,
		LastName:   o.LastName,
		Email:      o.Email,
		Phone:      o.Phone,
		TotalPrice: o.TotalPrice,
		TTNNumber:  o.TTNNumber,
		CreatedAt:  o.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}

	if o.TTNCreatedAt != nil {
		resp.TTNCreatedAt = o.TTNCreatedAt.Format("2006-01-02T15:04:05Z")
	}

	if o.Status.Name != nil {
		if name, ok := o.Status.Name["uk"]; ok {
			resp.Status.Name = name
		}
	}

	// Items
	items := make([]ManagerItemDTO, len(o.Items))
	for i, item := range o.Items {
		dto := ManagerItemDTO{
			ID:          item.ID,
			VariationID: item.VariationID,
			Price:       item.Price,
			Quantity:    item.Quantity,
			TotalPrice:  item.TotalPrice,
		}

		for _, img := range item.Variation.Product.Images {
			if img.IsPrimary {
				dto.ImageURL = img.ImageURL
				break
			}
			if dto.ImageURL == "" {
				dto.ImageURL = img.ImageURL
			}
		}

		for _, trans := range item.Variation.Product.Translations {
			if trans.LanguageCode == "uk" {
				dto.ProductName = trans.Name
				break
			}
		}

		items[i] = dto
	}
	resp.Items = items

	// Delivery
	if o.Delivery != nil {
		resp.Delivery = &DeliveryDTO{
			Provider:       o.Delivery.Provider,
			DeliveryType:   o.Delivery.DeliveryType,
			CityName:       o.Delivery.CityName,
			WarehouseName:  o.Delivery.WarehouseName,
			TrackingNumber: o.Delivery.TrackingNumber,
			Status:         o.Delivery.Status,
		}
	}

	return resp
}

// handleManagerError маппить доменні помилки на HTTP статуси
func (h *ManagerHandler) handleManagerError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidManagerToken):
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error: "Unauthorized", Message: "Invalid or expired manager token",
		})
	case errors.Is(err, domain.ErrOrderNotFound):
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: "Not Found", Message: "Order not found",
		})
	case errors.Is(err, domain.ErrOrderNotPaid):
		c.JSON(http.StatusUnprocessableEntity, ErrorResponse{
			Error: "Unprocessable Entity", Message: "Order must be in paid status",
		})
	case errors.Is(err, domain.ErrOrderNotProcessing):
		c.JSON(http.StatusUnprocessableEntity, ErrorResponse{
			Error: "Unprocessable Entity", Message: "Order must be in processing status",
		})
	case errors.Is(err, domain.ErrOrderAlreadyCancelled):
		c.JSON(http.StatusConflict, ErrorResponse{
			Error: "Conflict", Message: "Order is already cancelled",
		})
	case errors.Is(err, domain.ErrCancelBlockedAfterTTN):
		c.JSON(http.StatusConflict, ErrorResponse{
			Error: "Conflict", Message: "Cannot cancel order after TTN is created",
		})
	default:
		h.l.Errorw("unhandled manager error", "error", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Internal Server Error", Message: "An unexpected error occurred",
		})
	}
}
