package http

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	mymiddleware "github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/product/domain"
	validationerrors "github.com/VladHrytsaiuk/ecommerce-core/internal/shared/errors"
)

type AttributeHandler struct {
	service domain.AttributeService
	l       logger.Logger
}

func NewAttributeHandler(s domain.AttributeService, l logger.Logger) *AttributeHandler {
	return &AttributeHandler{service: s, l: l}
}

// GetAttributes godoc
// @Summary      Get attributes
// @Description  Get a list of all attributes
// @Tags         Products
// @Produce      json
// @Param        lang path string true "Language code (uk, en)"
// @Success      200      {array}  AttributeResponse
// @Failure      500      {object} ErrorResponse
// @Router       /api/{lang}/attributes [get]
func (h *AttributeHandler) GetAttributes(c *gin.Context) {
	lang := mymiddleware.GetLanguage(c)
	attributes, err := h.service.GetAll(c.Request.Context(), lang)
	if err != nil {
		h.l.Errorw("Failed to get attributes list", "err", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Internal Server Error", Message: err.Error()})
		return
	}

	c.JSON(http.StatusOK, mapAttributeListToResponse(attributes))
}

// AttrValueOptionResponse — значення характеристики (код + локалізована мітка) для підказок в адмінці.
type AttrValueOptionResponse struct {
	Code  string `json:"code"`
	Label string `json:"label"`
	Count int    `json:"count"`
}

// GetAttributeValues godoc
// @Summary      Get distinct attribute values
// @Description  Get all distinct values used for an attribute by its code (e.g. "type"), regardless of product active status. Powers admin product-form suggestions (Admin only).
// @Tags         Admin Attributes
// @Produce      json
// @Security     bearerAuth
// @Param        code query string true  "Attribute code (e.g. type)"
// @Param        lang query string false "Language code (uk, en). Default: uk"
// @Success      200  {array}   AttrValueOptionResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /api/admin/attributes/values [get]
func (h *AttributeHandler) GetAttributeValues(c *gin.Context) {
	code := strings.TrimSpace(c.Query("code"))
	if code == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Bad Request", Message: "query param 'code' is required"})
		return
	}

	lang := c.Query("lang")
	if lang != "uk" && lang != "en" {
		lang = "uk"
	}

	values, err := h.service.GetValues(c.Request.Context(), code, lang)
	if err != nil {
		h.l.Errorw("Failed to get attribute values", "err", err, "code", code)
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Internal Server Error", Message: err.Error()})
		return
	}

	res := make([]AttrValueOptionResponse, 0, len(values))
	for _, v := range values {
		res = append(res, AttrValueOptionResponse{Code: v.Code, Label: v.Label, Count: v.Count})
	}
	c.JSON(http.StatusOK, res)
}

// CreateAttribute godoc
// @Summary      Create attribute
// @Description  Create a new product attribute (Admin only)
// @Tags         Admin Attributes
// @Accept       json
// @Produce      json
// @Security     bearerAuth
// @Param        body body CreateAttributeRequest true "Attribute body"
// @Success      201      {object}  AttributeResponse
// @Failure      400      {object}  ErrorResponse
// @Failure      500      {object}  ErrorResponse
// @Router       /api/admin/attributes [post]
func (h *AttributeHandler) CreateAttribute(c *gin.Context) {
	var req CreateAttributeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Bad Request", Message: validationerrors.FormatValidationError(err)})
		return
	}

	attribute := &domain.Attribute{
		Code:              req.Code,
		SortOrder:         req.SortOrder,
		IsFilterable:      req.IsFilterable,
		IsVariantSpecific: req.IsVariantSpecific,
		UnitID:            req.UnitID,
		Translations: []domain.AttributeTranslation{
			{LanguageCode: "uk", Name: req.NameUk},
			{LanguageCode: "en", Name: req.NameEn},
		},
	}

	if err := h.service.Create(c.Request.Context(), attribute); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Internal Server Error", Message: err.Error()})
		return
	}

	c.JSON(http.StatusCreated, mapAttributeToResponse(*attribute))
}

// UpdateAttribute godoc
// @Summary      Update attribute
// @Description  Update an existing attribute (Admin only)
// @Tags         Admin Attributes
// @Accept       json
// @Produce      json
// @Security     bearerAuth
// @Param        id   path int true "Attribute ID"
// @Param        body body UpdateAttributeRequest true "Attribute body"
// @Success      200      {object}  AttributeResponse
// @Failure      400      {object}  ErrorResponse
// @Failure      404      {object}  ErrorResponse
// @Failure      500      {object}  ErrorResponse
// @Router       /api/admin/attributes/{id} [patch]
func (h *AttributeHandler) UpdateAttribute(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Bad Request", Message: "Invalid ID"})
		return
	}

	// Зчитуємо тіло для аналізу null vs absent (для unit_id)
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Bad Request", Message: "Failed to read request body"})
		return
	}

	var req UpdateAttributeRequest
	if err := json.Unmarshal(body, &req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Bad Request", Message: err.Error()})
		return
	}

	attribute, err := h.service.GetByID(c.Request.Context(), id, "uk")
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "Not Found", Message: "Attribute not found"})
		return
	}

	// Оновлюємо лише надіслані поля
	if req.Code != nil {
		attribute.Code = *req.Code
	}
	if req.SortOrder != nil {
		attribute.SortOrder = *req.SortOrder
	}
	if req.IsFilterable != nil {
		attribute.IsFilterable = *req.IsFilterable
	}
	if req.IsVariantSpecific != nil {
		attribute.IsVariantSpecific = *req.IsVariantSpecific
	}

	// UnitID: розрізняємо "не надіслано" та "надіслано як null" через raw JSON
	var rawFields map[string]json.RawMessage
	if err := json.Unmarshal(body, &rawFields); err == nil {
		if _, unitIDSent := rawFields["unit_id"]; unitIDSent {
			attribute.UnitID = req.UnitID // nil якщо null, значення якщо задано
		}
	}

	// Оновлення перекладів
	if req.NameUk != nil || req.NameEn != nil {
		newTranslations := []domain.AttributeTranslation{}
		ukFound, enFound := false, false
		for _, t := range attribute.Translations {
			if t.LanguageCode == "uk" {
				if req.NameUk != nil {
					t.Name = *req.NameUk
				}
				ukFound = true
			} else if t.LanguageCode == "en" {
				if req.NameEn != nil {
					t.Name = *req.NameEn
				}
				enFound = true
			}
			newTranslations = append(newTranslations, t)
		}
		if !ukFound && req.NameUk != nil {
			newTranslations = append(newTranslations, domain.AttributeTranslation{LanguageCode: "uk", Name: *req.NameUk, AttributeID: id})
		}
		if !enFound && req.NameEn != nil {
			newTranslations = append(newTranslations, domain.AttributeTranslation{LanguageCode: "en", Name: *req.NameEn, AttributeID: id})
		}
		attribute.Translations = newTranslations
	}

	if err := h.service.Update(c.Request.Context(), attribute); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Internal Server Error", Message: err.Error()})
		return
	}

	c.JSON(http.StatusOK, mapAttributeToResponse(*attribute))
}

// DeleteAttribute godoc
// @Summary      Delete attribute
// @Description  Delete an attribute (Admin only)
// @Tags         Admin Attributes
// @Security     bearerAuth
// @Param        id path int true "Attribute ID"
// @Success      200      {object}  map[string]string
// @Failure      400      {object}  ErrorResponse
// @Failure      500      {object}  ErrorResponse
// @Router       /api/admin/attributes/{id} [delete]
func (h *AttributeHandler) DeleteAttribute(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Bad Request", Message: "Invalid ID"})
		return
	}

	if err := h.service.Delete(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Internal Server Error", Message: err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Attribute successfully deleted"})
}

// ReorderAttributesRequest масив ID атрибутів у бажаному порядку.
// @example {"ids": [10, 5, 8]}
type ReorderAttributesRequest struct {
	IDs []int `json:"ids" binding:"required" swaggertype:"array,number" example:"10,5,8"` // Масив ID атрибутів у бажаному порядку.
}

// ReorderAttributes godoc
// @Summary      Reorder attributes
// @Description  Update sort_order for multiple attributes at once (Admin only). The order of IDs in the array defines the new sequence (0, 1, 2...).
// @Tags         Admin Attributes
// @Accept       json
// @Produce      json
// @Security     bearerAuth
// @Param        body body ReorderAttributesRequest true "IDs in desired order. Example: {'ids': [10, 5, 8]}"
// @Success      200      {object}  map[string]string
// @Failure      400      {object}  ErrorResponse
// @Failure      500      {object}  ErrorResponse
// @Router       /api/admin/attributes/reorder [patch]
func (h *AttributeHandler) ReorderAttributes(c *gin.Context) {
	var req ReorderAttributesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Bad Request", Message: err.Error()})
		return
	}

	if err := h.service.UpdateOrder(c.Request.Context(), req.IDs); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Internal Server Error", Message: err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Attributes reordered successfully"})
}
