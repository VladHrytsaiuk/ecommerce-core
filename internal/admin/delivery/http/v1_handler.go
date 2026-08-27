package http

import (
	stderrors "errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	adminApp "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/application"
	adminDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/domain"
	catalogDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/apiresponse"
	shared "github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware"
	ordersDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/orders/domain"
	promosDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/promos/domain"
)

// RegisterV1Routes exposes all currently implemented admin facades through
// the versioned transport contract. Every mutating route is permission-gated
// and reaches the facade, which atomically appends the audit outbox event.
func RegisterV1Routes(g *gin.RouterGroup, authorizer adminDomain.Authorizer, promos PromosFacade, catalog *adminApp.CatalogAdminFacade, orders *adminApp.OrdersAdminFacade, renderer *apiresponse.ErrorRenderer) {
	if g == nil || renderer == nil {
		return
	}
	if promos != nil {
		g.POST("/promos", RequirePermissionV1(authorizer, adminApp.PermissionPromosWrite, renderer), createPromoV1(promos, renderer))
	}
	if catalog != nil {
		g.POST("/catalog/products", RequirePermissionV1(authorizer, adminApp.PermissionCatalogWrite, renderer), catalogProductV1(catalog, false, renderer))
		g.PUT("/catalog/products/:id", RequirePermissionV1(authorizer, adminApp.PermissionCatalogWrite, renderer), catalogProductV1(catalog, true, renderer))
		g.DELETE("/catalog/products/:id", RequirePermissionV1(authorizer, adminApp.PermissionCatalogWrite, renderer), deleteCatalogProductV1(catalog, renderer))
		g.POST("/catalog/categories", RequirePermissionV1(authorizer, adminApp.PermissionCatalogWrite, renderer), catalogCategoryV1(catalog, false, renderer))
		g.PUT("/catalog/categories/:id", RequirePermissionV1(authorizer, adminApp.PermissionCatalogWrite, renderer), catalogCategoryV1(catalog, true, renderer))
	}
	if orders != nil {
		g.POST("/orders/:id/cancel", RequirePermissionV1(authorizer, adminApp.PermissionOrdersWrite, renderer), cancelOrderV1(orders, renderer))
		if orders.HasStatusWorkflow() {
			g.POST("/orders/:id/transition", RequirePermissionV1(authorizer, adminApp.PermissionOrdersWrite, renderer), transitionOrderV1(orders, renderer))
			g.GET("/orders/:id/status-history", RequirePermissionV1(authorizer, "orders:workflow:read", renderer), orderStatusHistoryV1(orders, renderer))
			g.GET("/order-workflow", RequirePermissionV1(authorizer, "orders:workflow:read", renderer), orderWorkflowV1(orders, renderer))
			g.PUT("/order-workflow/statuses/:code", RequirePermissionV1(authorizer, "orders:workflow:write", renderer), saveOrderWorkflowStatusV1(orders, renderer))
			g.PUT("/order-workflow/transitions", RequirePermissionV1(authorizer, "orders:workflow:write", renderer), saveOrderWorkflowTransitionV1(orders, renderer))
		}
	}
}

// deleteCatalogProductV1 godoc
// @Summary Delete a catalog product (v1 admin)
// @Tags Admin v1
// @Produce json
// @Param id path string true "Product ID"
// @Success 204
// @Failure 400,401,403,422 {object} apiresponse.ProblemDetails
// @Router /api/v1/admin/catalog/products/{id} [delete]
func deleteCatalogProductV1(f *adminApp.CatalogAdminFacade, errors *apiresponse.ErrorRenderer) gin.HandlerFunc {
	return func(c *gin.Context) {
		actor, ok := shared.AuthenticatedUserID(c)
		if !ok {
			errors.Abort(c, apiresponse.Unauthenticated(nil))
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil || id == uuid.Nil {
			errors.Abort(c, apiresponse.InvalidPayload(err))
			return
		}
		eventKey, err := v1Idempotency(c)
		if err != nil {
			errors.Abort(c, apiresponse.InvalidPayload(err))
			return
		}
		if err := f.DeleteProduct(c.Request.Context(), adminApp.CatalogCommand{ActorUserID: actor, EventKey: eventKey, IPAddress: c.ClientIP()}, id); err != nil {
			errors.Abort(c, apiresponse.ValidationFailed(err))
			return
		}
		c.Status(http.StatusNoContent)
	}
}

// createPromoV1 godoc
// @Summary Create a promotion (v1 admin)
// @Tags Admin v1
// @Accept json
// @Produce json
// @Param request body createPromoRequest true "Promotion"
// @Success 201 {object} apiresponse.SuccessResponse
// @Failure 400,401,403,422 {object} apiresponse.ProblemDetails
// @Router /api/v1/admin/promos [post]
func createPromoV1(f PromosFacade, errors *apiresponse.ErrorRenderer) gin.HandlerFunc {
	return func(c *gin.Context) {
		actor, ok := shared.AuthenticatedUserID(c)
		if !ok {
			errors.Abort(c, apiresponse.Unauthenticated(nil))
			return
		}
		var request createPromoRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			errors.Abort(c, apiresponse.InvalidPayload(err))
			return
		}
		eventKey, err := v1Idempotency(c)
		if err != nil {
			errors.Abort(c, apiresponse.InvalidPayload(err))
			return
		}
		active := true
		if request.IsActive != nil {
			active = *request.IsActive
		}
		promo, err := f.Create(c.Request.Context(), adminApp.CreatePromoCommand{ActorUserID: actor, EventKey: eventKey, IPAddress: c.ClientIP(), Code: promosDomain.Code{Code: request.Code, DiscountType: request.DiscountType, DiscountValue: request.DiscountValue, Currency: request.Currency, IsActive: active, ValidUntil: request.ValidUntil, UsageLimit: request.UsageLimit}})
		if err != nil {
			errors.Abort(c, apiresponse.ValidationFailed(err))
			return
		}
		apiresponse.Success(c, http.StatusCreated, gin.H{"id": promo.ID, "code": promo.Code})
	}
}

// catalogProductV1 godoc
// @Summary Create or update a catalog product (v1 admin)
// @Tags Admin v1
// @Accept json
// @Produce json
// @Param id path string false "Product ID"
// @Param request body object true "Product"
// @Success 200,201 {object} apiresponse.SuccessResponse
// @Failure 400,401,403,422 {object} apiresponse.ProblemDetails
// @Router /api/v1/admin/catalog/products [post]
// @Router /api/v1/admin/catalog/products/{id} [put]
func catalogProductV1(f *adminApp.CatalogAdminFacade, update bool, errors *apiresponse.ErrorRenderer) gin.HandlerFunc {
	return func(c *gin.Context) {
		actor, ok := shared.AuthenticatedUserID(c)
		if !ok {
			errors.Abort(c, apiresponse.Unauthenticated(nil))
			return
		}
		var product catalogDomain.Product
		if err := c.ShouldBindJSON(&product); err != nil {
			errors.Abort(c, apiresponse.InvalidPayload(err))
			return
		}
		if update {
			id, err := uuid.Parse(c.Param("id"))
			if err != nil || id == uuid.Nil {
				errors.Abort(c, apiresponse.InvalidPayload(err))
				return
			}
			product.ID = id
		}
		eventKey, err := v1Idempotency(c)
		if err != nil {
			errors.Abort(c, apiresponse.InvalidPayload(err))
			return
		}
		command := adminApp.CatalogCommand{ActorUserID: actor, EventKey: eventKey, IPAddress: c.ClientIP()}
		if update {
			err = f.UpdateProduct(c, command, &product)
		} else {
			err = f.CreateProduct(c, command, &product)
		}
		if err != nil {
			if stderrors.Is(err, adminApp.ErrMediaAssetsNotReady) {
				errors.Abort(c, apiresponse.InvalidPayload(err))
				return
			}
			errors.Abort(c, apiresponse.ValidationFailed(err))
			return
		}
		status := http.StatusCreated
		if update {
			status = http.StatusOK
		}
		apiresponse.Success(c, status, product)
	}
}

// catalogCategoryV1 godoc
// @Summary Create or update a catalog category (v1 admin)
// @Tags Admin v1
// @Accept json
// @Produce json
// @Param id path string false "Category ID"
// @Param request body object true "Category"
// @Success 200,201 {object} apiresponse.SuccessResponse
// @Failure 400,401,403,422 {object} apiresponse.ProblemDetails
// @Router /api/v1/admin/catalog/categories [post]
// @Router /api/v1/admin/catalog/categories/{id} [put]
func catalogCategoryV1(f *adminApp.CatalogAdminFacade, update bool, errors *apiresponse.ErrorRenderer) gin.HandlerFunc {
	return func(c *gin.Context) {
		actor, ok := shared.AuthenticatedUserID(c)
		if !ok {
			errors.Abort(c, apiresponse.Unauthenticated(nil))
			return
		}
		var category catalogDomain.Category
		if err := c.ShouldBindJSON(&category); err != nil {
			errors.Abort(c, apiresponse.InvalidPayload(err))
			return
		}
		if update {
			id, err := uuid.Parse(c.Param("id"))
			if err != nil || id == uuid.Nil {
				errors.Abort(c, apiresponse.InvalidPayload(err))
				return
			}
			category.ID = id
		}
		eventKey, err := v1Idempotency(c)
		if err != nil {
			errors.Abort(c, apiresponse.InvalidPayload(err))
			return
		}
		command := adminApp.CatalogCommand{ActorUserID: actor, EventKey: eventKey, IPAddress: c.ClientIP()}
		if update {
			err = f.UpdateCategory(c, command, &category)
		} else {
			err = f.CreateCategory(c, command, &category)
		}
		if err != nil {
			errors.Abort(c, apiresponse.ValidationFailed(err))
			return
		}
		status := http.StatusCreated
		if update {
			status = http.StatusOK
		}
		apiresponse.Success(c, status, category)
	}
}

// cancelOrderV1 godoc
// @Summary Cancel a pending order (v1 admin)
// @Tags Admin v1
// @Accept json
// @Produce json
// @Param id path string true "Order ID"
// @Param request body cancelOrderRequest true "Cancellation reason"
// @Success 204
// @Failure 400,401,403,422 {object} apiresponse.ProblemDetails
// @Router /api/v1/admin/orders/{id}/cancel [post]
func cancelOrderV1(f *adminApp.OrdersAdminFacade, errors *apiresponse.ErrorRenderer) gin.HandlerFunc {
	return func(c *gin.Context) {
		actor, ok := shared.AuthenticatedUserID(c)
		if !ok {
			errors.Abort(c, apiresponse.Unauthenticated(nil))
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil || id == uuid.Nil {
			errors.Abort(c, apiresponse.InvalidPayload(err))
			return
		}
		var request cancelOrderRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			errors.Abort(c, apiresponse.InvalidPayload(err))
			return
		}
		eventKey, err := v1Idempotency(c)
		if err != nil {
			errors.Abort(c, apiresponse.InvalidPayload(err))
			return
		}
		if err = f.Cancel(c, actor, eventKey, id, c.ClientIP(), request.Reason); err != nil {
			errors.Abort(c, apiresponse.ValidationFailed(err))
			return
		}
		apiresponse.NoContent(c)
	}
}

type cancelOrderRequest struct {
	Reason string `json:"reason" binding:"required,max=2000"`
}

type transitionOrderRequest struct {
	ToStatusCode   string `json:"to_status_code" binding:"required,max=64"`
	Reason         string `json:"reason" binding:"max=2000"`
	TrackingNumber string `json:"tracking_number" binding:"max=128"`
}

// transitionOrderV1 godoc
// @Summary Transition an order through the configured operational workflow (v1 admin)
// @Tags Admin v1
// @Accept json
// @Produce json
// @Param id path string true "Order ID"
// @Param request body transitionOrderRequest true "Configured status transition"
// @Success 204
// @Failure 400,401,403,422 {object} apiresponse.ProblemDetails
// @Router /api/v1/admin/orders/{id}/transition [post]
func transitionOrderV1(f *adminApp.OrdersAdminFacade, errors *apiresponse.ErrorRenderer) gin.HandlerFunc {
	return func(c *gin.Context) {
		actor, ok := shared.AuthenticatedUserID(c)
		if !ok {
			errors.Abort(c, apiresponse.Unauthenticated(nil))
			return
		}
		orderID, err := uuid.Parse(c.Param("id"))
		if err != nil || orderID == uuid.Nil {
			errors.Abort(c, apiresponse.InvalidPayload(err))
			return
		}
		var request transitionOrderRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			errors.Abort(c, apiresponse.InvalidPayload(err))
			return
		}
		eventKey, err := v1Idempotency(c)
		if err != nil {
			errors.Abort(c, apiresponse.InvalidPayload(err))
			return
		}
		if err := f.Transition(c.Request.Context(), adminApp.OrderTransitionCommand{
			ActorUserID: actor, EventKey: eventKey, IPAddress: c.ClientIP(), OrderID: orderID,
			ToStatusCode: request.ToStatusCode, Reason: request.Reason, TrackingNumber: request.TrackingNumber,
		}); err != nil {
			errors.Abort(c, apiresponse.ValidationFailed(err))
			return
		}
		apiresponse.NoContent(c)
	}
}

// orderStatusHistoryV1 godoc
// @Summary Get immutable order status history (v1 admin)
// @Tags Admin v1
// @Produce json
// @Param id path string true "Order ID"
// @Success 200 {object} apiresponse.SuccessResponse
// @Failure 400,401,403,422 {object} apiresponse.ProblemDetails
// @Router /api/v1/admin/orders/{id}/status-history [get]
func orderStatusHistoryV1(f *adminApp.OrdersAdminFacade, errors *apiresponse.ErrorRenderer) gin.HandlerFunc {
	return func(c *gin.Context) {
		actor, ok := shared.AuthenticatedUserID(c)
		if !ok {
			errors.Abort(c, apiresponse.Unauthenticated(nil))
			return
		}
		orderID, err := uuid.Parse(c.Param("id"))
		if err != nil || orderID == uuid.Nil {
			errors.Abort(c, apiresponse.InvalidPayload(err))
			return
		}
		history, err := f.ListOrderStatusHistory(c.Request.Context(), actor, orderID)
		if err != nil {
			errors.Abort(c, apiresponse.ValidationFailed(err))
			return
		}
		apiresponse.Success(c, http.StatusOK, gin.H{"history": history})
	}
}

// orderWorkflowV1 godoc
// @Summary Get the configured order workflow (v1 admin)
// @Tags Admin v1
// @Produce json
// @Success 200 {object} apiresponse.SuccessResponse
// @Failure 401,403,422 {object} apiresponse.ProblemDetails
// @Router /api/v1/admin/order-workflow [get]
func orderWorkflowV1(f *adminApp.OrdersAdminFacade, errors *apiresponse.ErrorRenderer) gin.HandlerFunc {
	return func(c *gin.Context) {
		actor, ok := shared.AuthenticatedUserID(c)
		if !ok {
			errors.Abort(c, apiresponse.Unauthenticated(nil))
			return
		}
		statuses, transitions, err := f.ListWorkflowConfiguration(c.Request.Context(), actor)
		if err != nil {
			errors.Abort(c, apiresponse.ValidationFailed(err))
			return
		}
		apiresponse.Success(c, http.StatusOK, gin.H{"statuses": statuses, "transitions": transitions})
	}
}

type orderWorkflowStatusRequest struct {
	Name          string                  `json:"name" binding:"required,max=128"`
	Description   string                  `json:"description" binding:"max=4000"`
	Color         string                  `json:"color" binding:"max=32"`
	SortOrder     int                     `json:"sort_order"`
	Kind          ordersDomain.StatusKind `json:"kind" binding:"required"`
	IsInitial     bool                    `json:"is_initial"`
	IsTerminal    bool                    `json:"is_terminal"`
	CustomerLabel string                  `json:"customer_label" binding:"max=128"`
	Enabled       *bool                   `json:"enabled"`
}

// saveOrderWorkflowStatusV1 godoc
// @Summary Create or update an operational order status (v1 admin)
// @Tags Admin v1
// @Accept json
// @Produce json
// @Param code path string true "Status code"
// @Param request body orderWorkflowStatusRequest true "Order status definition"
// @Success 204
// @Failure 400,401,403,422 {object} apiresponse.ProblemDetails
// @Router /api/v1/admin/order-workflow/statuses/{code} [put]
func saveOrderWorkflowStatusV1(f *adminApp.OrdersAdminFacade, errors *apiresponse.ErrorRenderer) gin.HandlerFunc {
	return func(c *gin.Context) {
		actor, ok := shared.AuthenticatedUserID(c)
		if !ok {
			errors.Abort(c, apiresponse.Unauthenticated(nil))
			return
		}
		var request orderWorkflowStatusRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			errors.Abort(c, apiresponse.InvalidPayload(err))
			return
		}
		key, err := v1Idempotency(c)
		if err != nil {
			errors.Abort(c, apiresponse.InvalidPayload(err))
			return
		}
		enabled := true
		if request.Enabled != nil {
			enabled = *request.Enabled
		}
		err = f.SaveStatusDefinition(c.Request.Context(), adminApp.OrderStatusDefinitionCommand{ActorUserID: actor, EventKey: key, IPAddress: c.ClientIP(), Definition: ordersDomain.OrderStatusDefinition{Code: c.Param("code"), Name: request.Name, Description: request.Description, Color: request.Color, SortOrder: request.SortOrder, Kind: request.Kind, IsInitial: request.IsInitial, IsTerminal: request.IsTerminal, CustomerLabel: request.CustomerLabel, Enabled: enabled}})
		if err != nil {
			errors.Abort(c, apiresponse.ValidationFailed(err))
			return
		}
		apiresponse.NoContent(c)
	}
}

type orderWorkflowTransitionRequest struct {
	FromStatusCode         string                           `json:"from_status_code" binding:"required,max=64"`
	ToStatusCode           string                           `json:"to_status_code" binding:"required,max=64"`
	AllowedTriggers        []ordersDomain.TransitionTrigger `json:"allowed_triggers" binding:"required,min=1,max=5"`
	RequiresPayment        bool                             `json:"requires_payment"`
	RequiresTrackingNumber bool                             `json:"requires_tracking_number"`
	RequiresReason         bool                             `json:"requires_reason"`
	RequiredPermission     string                           `json:"required_permission" binding:"max=128"`
}

// saveOrderWorkflowTransitionV1 godoc
// @Summary Create or update an order workflow transition (v1 admin)
// @Tags Admin v1
// @Accept json
// @Produce json
// @Param request body orderWorkflowTransitionRequest true "Order status transition"
// @Success 204
// @Failure 400,401,403,422 {object} apiresponse.ProblemDetails
// @Router /api/v1/admin/order-workflow/transitions [put]
func saveOrderWorkflowTransitionV1(f *adminApp.OrdersAdminFacade, errors *apiresponse.ErrorRenderer) gin.HandlerFunc {
	return func(c *gin.Context) {
		actor, ok := shared.AuthenticatedUserID(c)
		if !ok {
			errors.Abort(c, apiresponse.Unauthenticated(nil))
			return
		}
		var request orderWorkflowTransitionRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			errors.Abort(c, apiresponse.InvalidPayload(err))
			return
		}
		key, err := v1Idempotency(c)
		if err != nil {
			errors.Abort(c, apiresponse.InvalidPayload(err))
			return
		}
		err = f.SaveStatusTransition(c.Request.Context(), adminApp.OrderStatusTransitionCommand{ActorUserID: actor, EventKey: key, IPAddress: c.ClientIP(), Transition: ordersDomain.OrderStatusTransition{FromStatusCode: request.FromStatusCode, ToStatusCode: request.ToStatusCode, AllowedTriggers: request.AllowedTriggers, RequiresPayment: request.RequiresPayment, RequiresTrackingNumber: request.RequiresTrackingNumber, RequiresReason: request.RequiresReason, RequiredPermission: request.RequiredPermission}})
		if err != nil {
			errors.Abort(c, apiresponse.ValidationFailed(err))
			return
		}
		apiresponse.NoContent(c)
	}
}

func v1Idempotency(c *gin.Context) (uuid.UUID, error) {
	if value := strings.TrimSpace(c.GetHeader("Idempotency-Key")); value != "" {
		return uuid.Parse(value)
	}
	return uuid.New(), nil
}
