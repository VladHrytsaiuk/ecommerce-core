package http

import (
	stdhttp "net/http"

	"github.com/gin-gonic/gin"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/apiresponse"
	shared "github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware"
	ordersDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/orders/domain"
)

type OrdersV1Handler struct {
	service ordersDomain.Service
	errors  *apiresponse.ErrorRenderer
}

func RegisterV1Routes(group *gin.RouterGroup, service ordersDomain.Service, renderer *apiresponse.ErrorRenderer) {
	if group == nil || service == nil || renderer == nil {
		return
	}
	h := &OrdersV1Handler{service: service, errors: renderer}
	group.GET("", h.ListMine)
}

// ListMine godoc
// @Summary List current customer's orders (v1)
// @Tags Orders v1
// @Produce json
// @Param page query int false "Page number" default(1) minimum(1) maximum(1000)
// @Param limit query int false "Page size" default(20) minimum(1) maximum(100)
// @Success 200 {object} apiresponse.PaginatedResponse
// @Failure 401 {object} apiresponse.ProblemDetails
// @Router /api/v1/orders [get]
func (h *OrdersV1Handler) ListMine(c *gin.Context) {
	userID, ok := shared.AuthenticatedUserID(c)
	if !ok {
		h.errors.Abort(c, apiresponse.Unauthenticated(nil))
		return
	}
	page, limit := 1, 20
	if value, ok := queryInt(c, "page", 1, 1, 1000); ok {
		page = value
	} else {
		h.errors.Abort(c, apiresponse.InvalidPayload(nil))
		return
	}
	if value, ok := queryInt(c, "limit", 20, 1, 100); ok {
		limit = value
	} else {
		h.errors.Abort(c, apiresponse.InvalidPayload(nil))
		return
	}
	result, err := h.service.ListByCustomer(c.Request.Context(), userID, page, limit)
	if err != nil {
		h.errors.Abort(c, err)
		return
	}
	data := make([]gin.H, 0, len(result.Orders))
	for _, order := range result.Orders {
		data = append(data, gin.H{"id": order.ID, "number": order.Number, "status": order.Status, "total": order.Total, "created_at": order.CreatedAt})
	}
	totalPages := int((result.Total + int64(limit) - 1) / int64(limit))
	if totalPages == 0 {
		totalPages = 1
	}
	apiresponse.Paginated(c, stdhttp.StatusOK, data, apiresponse.PageMetadata{Page: page, Limit: limit, Total: result.Total, TotalPages: totalPages, HasNext: page < totalPages, HasPrevious: page > 1})
}
