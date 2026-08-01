package http

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/helpers"
	mymiddleware "github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/order/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	validationerrors "github.com/VladHrytsaiuk/ecommerce-core/internal/shared/errors"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/pagination"
	userDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/user/domain"
)

// OrderHandler обробляє HTTP запити для замовлень
type OrderHandler struct {
	service domain.OrderService
	l       logger.Logger
}

// NewOrderHandler створює новий інстанс хендлера
func NewOrderHandler(s domain.OrderService, l logger.Logger) *OrderHandler {
	return &OrderHandler{service: s, l: l}
}

// CreateOrder godoc
// @Summary      Create a new order
// @Description  Creates an order from the current cart. Supports both authenticated users and guests.
// @Tags         Orders
// @Accept       json
// @Produce      json
// @Param        lang path string true "Language code (uk, en)"
// @Param        body body CreateOrderRequest true "Order details"
// @Security     bearerAuth
// @Success      201  {object} CreateOrderResponse
// @Failure      400  {object} ErrorResponse
// @Failure      422  {object} ErrorResponse
// @Failure      500  {object} ErrorResponse
// @Router       /api/{lang}/orders [post]
func (h *OrderHandler) CreateOrder(c *gin.Context) {
	lang := mymiddleware.GetLanguage(c)

	var req CreateOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Bad Request",
			Message: "Invalid request body: delivery information is required",
		})
		return
	}

	userID, sessionID := helpers.GetIdentifiers(c)

	input := domain.CreateOrderInput{
		CustomerEmail:         req.Customer.Email,
		CustomerFirstName:     req.Customer.FirstName,
		CustomerLastName:      req.Customer.LastName,
		CustomerPhone:         req.Customer.Phone,
		DeliveryProvider:      req.Delivery.Provider,
		DeliveryType:          req.Delivery.DeliveryType,
		DeliveryCityRef:       req.Delivery.CityRef,
		DeliveryCityName:      req.Delivery.CityName,
		DeliveryWarehouseRef:  req.Delivery.WarehouseRef,
		DeliveryWarehouseName: req.Delivery.WarehouseName,
		AdminComment:          req.AdminComment,
		PayTypes:              req.PayTypes,
	}

	result, err := h.service.CreateOrder(c.Request.Context(), userID, sessionID, lang, input)
	if err != nil {
		h.handleOrderError(c, err)
		return
	}

	c.JSON(http.StatusCreated, CreateOrderResponse{
		OrderID:     result.OrderID,
		OrderNumber: result.OrderNumber,
		TotalPrice:  result.TotalPrice,
		Status:      result.Status,
		PaymentURL:  result.PaymentURL,
		IsNewUser:   result.IsNewUser,
		SetupToken:  result.SetupToken,
	})
}

// GetOrder godoc
// @Summary      Get order by ID
// @Description  Returns order details by UUID
// @Tags         Orders
// @Produce      json
// @Param        lang path string true "Language code (uk, en)"
// @Param        id path string true "Order UUID"
// @Security     bearerAuth
// @Success      200  {object} OrderResponse
// @Failure      404  {object} ErrorResponse
// @Failure      500  {object} ErrorResponse
// @Router       /api/{lang}/orders/{id} [get]
func (h *OrderHandler) GetOrder(c *gin.Context) {
	orderIDStr := c.Param("id")
	orderID, err := uuid.Parse(orderIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Bad Request",
			Message: "Invalid order ID format",
		})
		return
	}

	order, err := h.service.GetByID(c.Request.Context(), orderID)
	if err != nil {
		if errors.Is(err, domain.ErrOrderNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Error:   "Not Found",
				Message: "Order not found",
			})
			return
		}
		h.l.Errorw("failed to get order", "error", err, "order_id", orderID)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "Internal Server Error",
			Message: "Failed to retrieve order",
		})
		return
	}

	// Security check:
	// - Admins can view any order.
	// - Authenticated customers can only view their own orders.
	// - Guests (unauthenticated) can view by unguessable UUID.
	userID, _ := helpers.GetIdentifiers(c)
	var roleID int
	if roleIDRaw, exists := c.Get("role_id"); exists {
		if r, ok := roleIDRaw.(int); ok {
			roleID = r
		}
	}

	if userID != nil && roleID != userDomain.RoleAdmin {
		if order.UserID == nil || *order.UserID != *userID {
			c.JSON(http.StatusForbidden, ErrorResponse{
				Error:   "Forbidden",
				Message: "You do not have permission to view this order",
			})
			return
		}
	}

	lang := mymiddleware.GetLanguage(c)
	orderDTO := mapToOrderResponse(order, lang)

	// Якщо статус "Очікує на оплату", генеруємо посилання на оплату
	if order.StatusID == domain.StatusPendingPayment {
		paymentURL, err := h.service.GeneratePaymentURL(c.Request.Context(), order.ID)
		if err == nil {
			orderDTO.PaymentURL = paymentURL
		} else {
			h.l.Errorw("failed to generate payment URL for GetOrder", "error", err, "order_id", order.ID)
		}
	}

	c.JSON(http.StatusOK, orderDTO)
}

// GetPaymentStatus godoc
// @Summary      Get payment status
// @Description  Lightweight endpoint to poll payment status by order ID. Returns status as "pending", "paid", or "failed".
// @Tags         Orders
// @Produce      json
// @Param        lang path string true "Language code (uk, en)"
// @Param        id path string true "Order UUID"
// @Success      200  {object} OrderPaymentStatusResponse
// @Failure      400  {object} ErrorResponse
// @Failure      404  {object} ErrorResponse
// @Failure      500  {object} ErrorResponse
// @Router       /api/{lang}/orders/{id}/payment-status [get]
func (h *OrderHandler) GetPaymentStatus(c *gin.Context) {
	orderIDStr := c.Param("id")
	orderID, err := uuid.Parse(orderIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Bad Request",
			Message: "Invalid order ID format",
		})
		return
	}

	statusID, err := h.service.GetOrderStatusByID(c.Request.Context(), orderID)
	if err != nil {
		if errors.Is(err, domain.ErrOrderNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Error:   "Not Found",
				Message: "Order not found",
			})
			return
		}
		h.l.Errorw("failed to get order status", "error", err, "order_id", orderID)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "Internal Server Error",
			Message: "Failed to retrieve order status",
		})
		return
	}

	statusStr := "pending"
	if statusID >= domain.StatusPaid && statusID <= domain.StatusDelivered {
		statusStr = "paid"
	} else if statusID == domain.StatusCancelled || statusID == domain.StatusRefunded {
		statusStr = "failed"
	}

	c.JSON(http.StatusOK, gin.H{
		"status": statusStr,
	})
}

// GetMyOrders godoc
// @Summary      Get my orders
// @Description  Returns list of orders for the authenticated user
// @Tags         Orders
// @Produce      json
// @Param        lang path string true "Language code (uk, en)"
// @Param        page query int false "Page number"
// @Param        per_page query int false "Items per page"
// @Security     bearerAuth
// @Success      200  {object} map[string]interface{}
// @Failure      401  {object} ErrorResponse
// @Failure      500  {object} ErrorResponse
// @Router       /api/{lang}/orders/my [get]
func (h *OrderHandler) GetMyOrders(c *gin.Context) {
	userIDRaw, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "Unauthorized",
			Message: "Authentication required",
		})
		return
	}

	var userID uuid.UUID
	switch v := userIDRaw.(type) {
	case string:
		userID, _ = uuid.Parse(v)
	case uuid.UUID:
		userID = v
	}

	if userID == uuid.Nil {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "Unauthorized",
			Message: "Invalid user token",
		})
		return
	}

	pgn := pagination.Params{}
	if err := c.ShouldBindQuery(&pgn); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Bad Request",
			Message: validationerrors.FormatValidationError(err),
		})
		return
	}

	orders, meta, err := h.service.GetMyOrders(c.Request.Context(), userID, pgn)
	if err != nil {
		h.l.Errorw("failed to get user orders", "error", err, "user_id", userID)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "Internal Server Error",
			Message: "Failed to retrieve orders",
		})
		return
	}

	lang := mymiddleware.GetLanguage(c)
	orderDTOs := make([]OrderBriefResponse, len(orders))
	for i, o := range orders {
		orderDTOs[i] = mapToOrderBriefResponse(&o, lang)
		// Якщо статус "Очікує на оплату", генеруємо посилання на оплату
		if o.StatusID == domain.StatusPendingPayment {
			paymentURL, err := h.service.GeneratePaymentURL(c.Request.Context(), o.ID)
			if err == nil {
				orderDTOs[i].PaymentURL = paymentURL
			} else {
				h.l.Errorw("failed to generate payment URL for GetMyOrders", "error", err, "order_id", o.ID)
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"data": orderDTOs,
		"meta": meta,
	})
}

// CancelOrder godoc
// @Summary      Cancel my order
// @Description  Cancels an order. Only allowed for the owner of the order before TTN is created.
// @Tags         Orders
// @Produce      json
// @Param        lang path string true "Language code (uk, en)"
// @Param        id path string true "Order UUID"
// @Security     bearerAuth
// @Success      200  {object} map[string]interface{}
// @Failure      400  {object} ErrorResponse
// @Failure      401  {object} ErrorResponse
// @Failure      403  {object} ErrorResponse
// @Failure      422  {object} ErrorResponse
// @Failure      500  {object} ErrorResponse
// @Router       /api/{lang}/orders/{id}/cancel [post]
func (h *OrderHandler) CancelOrder(c *gin.Context) {
	orderIDStr := c.Param("id")
	orderID, err := uuid.Parse(orderIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Bad Request",
			Message: "Invalid order ID format",
		})
		return
	}

	userIDRaw, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "Unauthorized",
			Message: "Authentication required",
		})
		return
	}

	var userID uuid.UUID
	switch v := userIDRaw.(type) {
	case string:
		userID, _ = uuid.Parse(v)
	case uuid.UUID:
		userID = v
	}

	if userID == uuid.Nil {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "Unauthorized",
			Message: "Invalid user token",
		})
		return
	}

	if err := h.service.CancelOrderByUser(c.Request.Context(), userID, orderID); err != nil {
		if errors.Is(err, domain.ErrOrderNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Error:   "Not Found",
				Message: "Order not found",
			})
			return
		}
		if errors.Is(err, domain.ErrTTNAlreadyCreated) {
			c.JSON(http.StatusUnprocessableEntity, ErrorResponse{
				Error:   "Unprocessable Entity",
				Message: "Cannot cancel order because delivery (TTN) is already created",
			})
			return
		}
		if errors.Is(err, domain.ErrOrderAlreadyCancelled) {
			c.JSON(http.StatusUnprocessableEntity, ErrorResponse{
				Error:   "Unprocessable Entity",
				Message: "Order is already cancelled or refunded",
			})
			return
		}
		if err.Error() == "unauthorized to cancel this order" {
			c.JSON(http.StatusForbidden, ErrorResponse{
				Error:   "Forbidden",
				Message: "You do not have permission to cancel this order",
			})
			return
		}
		h.l.Errorw("failed to cancel order", "error", err, "order_id", orderID)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "Internal Server Error",
			Message: "Failed to cancel order",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Order successfully cancelled",
	})
}

// mapToOrderBriefResponse маппить доменну модель у DTO
func mapToOrderBriefResponse(o *domain.Order, lang string) OrderBriefResponse {
	resp := OrderBriefResponse{
		ID:          o.ID,
		OrderNumber: o.OrderNumber,
		CreatedAt:   o.CreatedAt.Format("2006-01-02T15:04:05Z"),
		TotalPrice:  o.TotalPrice,
		Status: OrderStatusDTO{
			ID:   o.Status.ID,
			Code: o.Status.Code,
		},
	}

	if o.Status.Name != nil {
		if name, ok := o.Status.Name[lang]; ok {
			resp.Status.Name = name
		} else if name, ok := o.Status.Name["uk"]; ok {
			resp.Status.Name = name
		}
	}

	return resp
}

func mapToOrderResponse(o *domain.Order, lang string) OrderResponse {
	resp := OrderResponse{
		ID:          o.ID,
		OrderNumber: o.OrderNumber,
		Status: OrderStatusDTO{
			ID:   o.Status.ID,
			Code: o.Status.Code,
		},
		FirstName:      o.FirstName,
		LastName:       o.LastName,
		Email:          o.Email,
		Phone:          o.Phone,
		TotalPrice:     o.TotalPrice,
		PromoCode:      o.PromoCode,
		DiscountAmount: o.DiscountAmount,
		AdminComment:   o.AdminComment,
		CreatedAt:      o.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}

	if o.Status.Name != nil {
		if name, ok := o.Status.Name[lang]; ok {
			resp.Status.Name = name
		} else if name, ok := o.Status.Name["uk"]; ok {
			resp.Status.Name = name
		}
	}

	// Items
	items := make([]OrderItemDTO, len(o.Items))
	for i, item := range o.Items {
		slug := item.Variation.Slug
		if slug == "" {
			if len(item.Variation.Product.Translations) > 0 {
				slug = item.Variation.Product.Translations[0].Slug
			}
		}
		dto := OrderItemDTO{
			ID:          item.ID,
			VariationID:     item.VariationID,
			Slug:            slug,
			Price:           item.Price,
			Quantity:        item.Quantity,
			TotalPrice:      item.TotalPrice,
			DiscountAmount:  item.DiscountAmount,
			FinalTotalPrice: item.FinalTotalPrice,
		}

		// Extract Image
		for _, img := range item.Variation.Product.Images {
			if img.IsPrimary {
				dto.ImageURL = img.ImageURL
				break
			}
			if dto.ImageURL == "" { // Fallback to first image
				dto.ImageURL = img.ImageURL
			}
		}

		// Extract Name
		for _, trans := range item.Variation.Product.Translations {
			if trans.LanguageCode == lang {
				dto.ProductName = trans.Name
				break
			}
		}
		if dto.ProductName == "" && len(item.Variation.Product.Translations) > 0 {
			for _, trans := range item.Variation.Product.Translations {
				if trans.LanguageCode == "uk" {
					dto.ProductName = trans.Name
					break
				}
			}
			if dto.ProductName == "" {
				dto.ProductName = item.Variation.Product.Translations[0].Name
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

// handleOrderError маппить доменні помилки на HTTP статуси
func (h *OrderHandler) handleOrderError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrEmptyCart):
		c.JSON(http.StatusUnprocessableEntity, ErrorResponse{
			Error: "Unprocessable Entity", Message: "Cart is empty",
		})
	case errors.Is(err, domain.ErrInactiveItem):
		c.JSON(http.StatusUnprocessableEntity, ErrorResponse{
			Error: "Unprocessable Entity", Message: "One or more items are no longer available",
		})
	case errors.Is(err, domain.ErrNoGuestData):
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "Bad Request", Message: "Guest checkout requires email, first_name, last_name, phone",
		})
	case errors.Is(err, domain.ErrNoDeliveryData):
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "Bad Request", Message: "Delivery information is required",
		})
	case errors.Is(err, domain.ErrNoIdentifier):
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "Bad Request", Message: "Authentication or session is required",
		})
	case errors.Is(err, userDomain.ErrPhoneNotVerified):
		c.JSON(http.StatusForbidden, ErrorResponse{
			Error: "Forbidden", Message: "Phone number is not verified",
		})
	case errors.Is(err, domain.ErrMinOrderAmountNotReached):
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "Bad Request", Message: "minimum order amount not reached",
		})
	default:
		h.l.Errorw("unhandled order error", "error", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Internal Server Error", Message: "Failed to create order",
		})
	}
}
