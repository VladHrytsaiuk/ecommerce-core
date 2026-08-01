package http

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shipment/domain"
)

// ShipmentHandler — HTTP-обробник для ендпоінтів доставки
type ShipmentHandler struct {
	service domain.ShipmentService
	l       logger.Logger
}

// NewShipmentHandler створює новий обробник доставки
func NewShipmentHandler(s domain.ShipmentService, l logger.Logger) *ShipmentHandler {
	return &ShipmentHandler{service: s, l: l}
}

// GetAreas godoc
// @Summary      Отримати список областей
// @Description  Повертає список усіх областей України від вказаного провайдера доставки
// @Tags         Shipment
// @Produce      json
// @Param        provider path string true "Назва провайдера (novaposhta, ukrposhta)"
// @Success      200 {array}  AreaResponse
// @Failure      400 {object} ErrorResponse
// @Failure      502 {object} ErrorResponse
// @Router       /api/shipment/{provider}/areas [get]
func (h *ShipmentHandler) GetAreas(c *gin.Context) {
	provider := c.Param("provider")

	areas, err := h.service.GetAreas(c.Request.Context(), provider)
	if err != nil {
		if errors.Is(err, domain.ErrUnknownProvider) {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error:   "Bad Request",
				Message: "Невідомий провайдер доставки: " + provider,
			})
			return
		}
		h.l.Errorw("Failed to get areas", "provider", provider, "err", err)
		c.JSON(http.StatusBadGateway, ErrorResponse{
			Error:   "Bad Gateway",
			Message: "Не вдалося отримати список областей від служби доставки",
		})
		return
	}

	c.JSON(http.StatusOK, mapAreasToResponse(areas))
}

// GetCities godoc
// @Summary      Отримати список міст
// @Description  Повертає список міст для вказаної області від провайдера доставки
// @Tags         Shipment
// @Produce      json
// @Param        provider path string true "Назва провайдера (novaposhta, ukrposhta)"
// @Param        area_ref query string true "Ref області"
// @Success      200 {array}  CityResponse
// @Failure      400 {object} ErrorResponse
// @Failure      502 {object} ErrorResponse
// @Router       /api/shipment/{provider}/cities [get]
func (h *ShipmentHandler) GetCities(c *gin.Context) {
	provider := c.Param("provider")

	areaRef := c.Query("area_ref")
	if areaRef == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Bad Request",
			Message: "Параметр area_ref є обов'язковим",
		})
		return
	}

	cities, err := h.service.GetCities(c.Request.Context(), provider, areaRef)
	if err != nil {
		if errors.Is(err, domain.ErrUnknownProvider) {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error:   "Bad Request",
				Message: "Невідомий провайдер доставки: " + provider,
			})
			return
		}
		h.l.Errorw("Failed to get cities", "provider", provider, "area_ref", areaRef, "err", err)
		c.JSON(http.StatusBadGateway, ErrorResponse{
			Error:   "Bad Gateway",
			Message: "Не вдалося отримати список міст від служби доставки",
		})
		return
	}

	c.JSON(http.StatusOK, mapCitiesToResponse(cities))
}

// GetWarehouses godoc
// @Summary      Отримати список відділень/поштоматів
// @Description  Повертає список відділень або поштоматів для вказаного міста від провайдера доставки
// @Tags         Shipment
// @Produce      json
// @Param        provider path string true "Назва провайдера (novaposhta, ukrposhta)"
// @Param        city_ref query string true "Ref міста"
// @Param        type     query string false "Тип: branch (відділення + вантажні), postomat (поштомат)"
// @Success      200 {array}  WarehouseResponse
// @Failure      400 {object} ErrorResponse
// @Failure      502 {object} ErrorResponse
// @Router       /api/shipment/{provider}/warehouses [get]
func (h *ShipmentHandler) GetWarehouses(c *gin.Context) {
	provider := c.Param("provider")

	cityRef := c.Query("city_ref")
	if cityRef == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Bad Request",
			Message: "Параметр city_ref є обов'язковим",
		})
		return
	}

	warehouseType := c.Query("type")

	warehouses, err := h.service.GetWarehouses(c.Request.Context(), provider, cityRef, warehouseType)
	if err != nil {
		if errors.Is(err, domain.ErrUnknownProvider) {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error:   "Bad Request",
				Message: "Невідомий провайдер доставки: " + provider,
			})
			return
		}
		h.l.Errorw("Failed to get warehouses", "provider", provider, "city_ref", cityRef, "type", warehouseType, "err", err)
		c.JSON(http.StatusBadGateway, ErrorResponse{
			Error:   "Bad Gateway",
			Message: "Не вдалося отримати список відділень від служби доставки",
		})
		return
	}

	c.JSON(http.StatusOK, mapWarehousesToResponse(warehouses))
}

// GetShippingConfig godoc
// @Summary      Отримати налаштування доставки
// @Description  Повертає глобальні налаштування доставки, зокрема поріг безкоштовної доставки
// @Tags         Shipment
// @Produce      json
// @Success      200 {object} ShippingConfigResponse
// @Failure      500 {object} ErrorResponse
// @Router       /api/shipment/config [get]
func (h *ShipmentHandler) GetShippingConfig(c *gin.Context) {
	threshold, err := h.service.GetFreeShippingThreshold(c.Request.Context())
	if err != nil {
		h.l.Errorw("Failed to get shipping threshold", "err", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "Internal Server Error",
			Message: "Не вдалося отримати налаштування доставки",
		})
		return
	}

	minOrder, err := h.service.GetMinimumOrderAmount(c.Request.Context())
	if err != nil {
		h.l.Errorw("Failed to get minimum order amount", "err", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "Internal Server Error",
			Message: "Не вдалося отримати налаштування мінімального замовлення",
		})
		return
	}

	c.JSON(http.StatusOK, ShippingConfigResponse{
		FreeShippingThreshold: threshold,
		MinOrderAmount:        minOrder,
	})
}

// UpdateShippingRule godoc
// @Summary      Оновити правило доставки
// @Description  Оновлює або створює правило доставки (напр. 'all' для безкоштовної доставки, 'min_order' для мін. суми замовлення)
// @Tags         Admin, Shipment
// @Accept       json
// @Produce      json
// @Param        body body UpdateShippingRuleRequest true "Дані правила"
// @Security     bearerAuth
// @Success      200 {object} map[string]string
// @Failure      400 {object} ErrorResponse
// @Failure      500 {object} ErrorResponse
// @Router       /api/admin/shipment/rules [post]
func (h *ShipmentHandler) UpdateShippingRule(c *gin.Context) {
	var req UpdateShippingRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Bad Request",
			Message: "Некоректні дані: provider та min_order_amount є обов'язковими",
		})
		return
	}

	if err := h.service.UpdateShippingRule(c.Request.Context(), req.Provider, req.MinOrderAmount); err != nil {
		h.l.Errorw("Failed to update shipping rule", "err", err, "provider", req.Provider)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "Internal Server Error",
			Message: "Не вдалося оновити правило доставки",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Правило доставки успішно оновлено"})
}
