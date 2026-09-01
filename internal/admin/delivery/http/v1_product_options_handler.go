package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	adminApp "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/application"
	catalogDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/apiresponse"
	shared "github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware"
)

type createProductOptionRequest struct {
	Name     string                     `json:"name" binding:"required,max=100"`
	Position int                        `json:"position" binding:"gte=0"`
	Values   []createProductOptionValue `json:"values" binding:"required,min=1,max=100"`
}
type createProductOptionValue struct {
	Value    string `json:"value" binding:"required,max=255"`
	Position int    `json:"position" binding:"gte=0"`
}

// createProductOptionV1 creates one option axis and all of its values in an
// audited Catalog transaction.
// @Summary Create a product option and its values (v1 admin)
// @Tags Admin v1
// @Accept json
// @Produce json
// @Param id path string true "Product ID"
// @Param request body createProductOptionRequest true "Option and values"
// @Success 201 {object} apiresponse.SuccessResponse
// @Router /api/v1/admin/products/{id}/options [post]
func createProductOptionV1(f *adminApp.CatalogAdminFacade, errors *apiresponse.ErrorRenderer) gin.HandlerFunc {
	return func(c *gin.Context) {
		actor, ok := shared.AuthenticatedUserID(c)
		if !ok {
			errors.Abort(c, apiresponse.Unauthenticated(nil))
			return
		}
		productID, err := uuid.Parse(c.Param("id"))
		if err != nil || productID == uuid.Nil {
			errors.Abort(c, apiresponse.InvalidPayload(err))
			return
		}
		var request createProductOptionRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			errors.Abort(c, apiresponse.InvalidPayload(err))
			return
		}
		key, err := v1Idempotency(c)
		if err != nil {
			errors.Abort(c, apiresponse.InvalidPayload(err))
			return
		}
		option := &catalogDomain.ProductOption{Name: request.Name, Position: request.Position, Values: make([]catalogDomain.ProductOptionValue, 0, len(request.Values))}
		for _, value := range request.Values {
			option.Values = append(option.Values, catalogDomain.ProductOptionValue{Value: value.Value, Position: value.Position})
		}
		if err := f.CreateProductOption(c.Request.Context(), adminApp.CatalogCommand{ActorUserID: actor, EventKey: key, IPAddress: c.ClientIP()}, productID, option); err != nil {
			errors.Abort(c, apiresponse.ValidationFailed(err))
			return
		}
		apiresponse.Success(c, http.StatusCreated, option)
	}
}

type createProductVariantRequest struct {
	SKU               string      `json:"sku"`
	Barcode           string      `json:"barcode"`
	Status            string      `json:"status"`
	PriceAmount       int64       `json:"price_amount" binding:"gte=0"`
	Currency          string      `json:"currency" binding:"required,len=3"`
	WeightGrams       int         `json:"weight_grams" binding:"gte=0"`
	InventoryQuantity int         `json:"inventory_quantity" binding:"gte=0"`
	OptionValueIDs    []uuid.UUID `json:"option_value_ids" binding:"required,min=1,max=32"`
}

// @Summary Create a sellable product variant (v1 admin)
// @Tags Admin v1
// @Accept json
// @Produce json
// @Param id path string true "Product ID"
// @Param request body createProductVariantRequest true "Variant"
// @Success 201 {object} apiresponse.SuccessResponse
// @Router /api/v1/admin/products/{id}/variants [post]
func createProductVariantV1(f *adminApp.CatalogAdminFacade, errors *apiresponse.ErrorRenderer) gin.HandlerFunc {
	return func(c *gin.Context) {
		actor, ok := shared.AuthenticatedUserID(c)
		if !ok {
			errors.Abort(c, apiresponse.Unauthenticated(nil))
			return
		}
		productID, err := uuid.Parse(c.Param("id"))
		if err != nil || productID == uuid.Nil {
			errors.Abort(c, apiresponse.InvalidPayload(err))
			return
		}
		var request createProductVariantRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			errors.Abort(c, apiresponse.InvalidPayload(err))
			return
		}
		price, err := money.NewMoney(request.PriceAmount, request.Currency)
		if err != nil {
			errors.Abort(c, apiresponse.ValidationFailed(err))
			return
		}
		key, err := v1Idempotency(c)
		if err != nil {
			errors.Abort(c, apiresponse.InvalidPayload(err))
			return
		}
		variant := &catalogDomain.ProductVariant{SKU: request.SKU, Barcode: request.Barcode, Status: request.Status, Price: price, WeightGrams: request.WeightGrams}
		if err := f.CreateProductVariant(c.Request.Context(), adminApp.CatalogCommand{ActorUserID: actor, EventKey: key, IPAddress: c.ClientIP()}, productID, variant, request.OptionValueIDs, request.InventoryQuantity); err != nil {
			errors.Abort(c, apiresponse.ValidationFailed(err))
			return
		}
		apiresponse.Success(c, http.StatusCreated, variant)
	}
}
