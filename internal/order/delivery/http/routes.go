//go:build legacy
// +build legacy

package http

import (
	"github.com/VladHrytsaiuk/ecommerce-core/internal/order/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/gin-gonic/gin"
)

// RegisterOrderRoutes реєструє маршрути модуля замовлень.
//
// - POST /orders — створення замовлення (доступне і авторизованим, і анонімним через session)
// - GET /orders/my — мої замовлення (тільки авторизовані)
// - GET /orders/:id — деталі замовлення (тільки авторизовані)
func RegisterOrderRoutes(
	optionalAuthGrp *gin.RouterGroup,
	authGrp *gin.RouterGroup,
	s domain.OrderService,
	l logger.Logger,
) {
	h := NewOrderHandler(s, l)

	// Створення замовлення — доступне і для гостей (OptionalAuth + Session)
	optionalAuthGrp.POST("/orders", h.CreateOrder)

	// Отримання замовлень — тільки для авторизованих
	authGrp.GET("/orders/my", h.GetMyOrders)

	// Скасування замовлення користувачем (авторизація через bearer auth, перевірка власника всередині)
	authGrp.POST("/orders/:id/cancel", h.CancelOrder)

	// Опитування статусу оплати (публічний доступ за UUID)
	optionalAuthGrp.GET("/orders/:id", h.GetOrder)
	optionalAuthGrp.GET("/orders/:id/payment-status", h.GetPaymentStatus)
}

// RegisterManagerRoutes реєструє маршрути менеджерського доступу до замовлень.
// Авторизація відбувається через manager token в query параметрі, не через JWT.
//
// API:
//   - GET  /api/manager/orders/:orderNumber    — отримати замовлення
//   - POST /api/manager/orders/:orderNumber/confirm — підтвердити замовлення
//   - POST /api/manager/orders/:orderNumber/cancel  — скасувати замовлення
//
// HTML:
//   - GET /manager/orders/:orderNumber — lightweight manager page
func RegisterManagerRoutes(
	r *gin.Engine,
	managerSvc domain.ManagerService,
	l logger.Logger,
) {
	mh := NewManagerHandler(managerSvc, l)
	mph := NewManagerPageHandler(managerSvc, l)

	// Manager API (без JWT — валідація через token query param)
	managerAPI := r.Group("/api/manager/orders")
	{
		managerAPI.GET("/:orderNumber", mh.GetManagerOrder)
		managerAPI.POST("/:orderNumber/confirm", mh.ConfirmOrder)
		managerAPI.POST("/:orderNumber/cancel", mh.CancelOrder)
	}

	// Manager HTML page
	r.GET("/manager/orders/:orderNumber", mph.RenderManagerPage)
}

// RegisterAdminOrderRoutes реєструє маршрути для управління замовленнями в адмінці.
func RegisterAdminOrderRoutes(
	adminGrp *gin.RouterGroup,
	adminSvc domain.AdminOrderService,
	l logger.Logger,
) {
	h := NewAdminOrderHandler(adminSvc, l)

	adminGrp.GET("/orders", h.ListOrders)
	adminGrp.GET("/orders/statuses", h.GetStatuses)
	adminGrp.GET("/orders/:id", h.GetOrderDetails)
	adminGrp.PUT("/orders/:id/comment", h.UpdateComment)
	adminGrp.POST("/orders/:id/confirm", h.ConfirmOrder)
	adminGrp.POST("/orders/:id/cancel", h.CancelOrder)
}
