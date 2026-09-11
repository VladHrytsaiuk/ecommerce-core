package http

import (
	stderrors "errors"
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

type updateProductVariantRequest struct {
	SKU         string `json:"sku"`
	Barcode     string `json:"barcode"`
	Status      string `json:"status"`
	PriceAmount int64  `json:"price_amount" binding:"gte=0"`
	Currency    string `json:"currency" binding:"required,len=3"`
	WeightGrams int    `json:"weight_grams" binding:"gte=0"`
}

// updateProductVariantV1 godoc
// @Summary Update a sellable product variant (v1 admin)
// @Description Option values are not editable: they define which variant this is, so changing them would turn an existing SKU into a different one while carts, reservations and order snapshots still referenced it. Existing orders keep their immutable price snapshot.
// @Tags Admin v1
// @Accept json
// @Produce json
// @Param variant_id path string true "Variant UUID"
// @Param request body updateProductVariantRequest true "Variant"
// @Param Idempotency-Key header string false "Reuses the audit identity of a retried request"
// @Success 200 {object} apiresponse.SuccessResponse
// @Failure 400,401,403,404,422 {object} apiresponse.ProblemDetails
// @Router /api/v1/admin/variants/{variant_id} [put]
func updateProductVariantV1(f *adminApp.CatalogAdminFacade, renderer *apiresponse.ErrorRenderer) gin.HandlerFunc {
	return func(c *gin.Context) {
		cmd, ok := contentCommand(c, renderer)
		if !ok {
			return
		}
		variantID, ok := contentID(c, renderer, "variant_id")
		if !ok {
			return
		}
		var request updateProductVariantRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			renderer.Abort(c, apiresponse.InvalidPayload(err))
			return
		}
		price, err := money.NewMoney(request.PriceAmount, request.Currency)
		if err != nil {
			renderer.Abort(c, apiresponse.InvalidPayload(err))
			return
		}
		variant, err := f.UpdateVariant(c.Request.Context(), cmd, variantID, catalogDomain.UpdateVariantCommand{
			SKU: request.SKU, Barcode: request.Barcode, Status: request.Status,
			Price: price, WeightGrams: request.WeightGrams,
		})
		if err != nil {
			renderer.Abort(c, variantError(err))
			return
		}
		apiresponse.Success(c, http.StatusOK, variant)
	}
}

// archiveProductVariantV1 godoc
// @Summary Withdraw a product variant from sale (v1 admin)
// @Description Archives rather than deletes. A row removal would be restricted by carts, silently cascade away wishlist and comparison entries, and orphan stock, reservations, returns and back-in-stock subscriptions, which reference variants without a foreign key.
// @Tags Admin v1
// @Produce json
// @Param variant_id path string true "Variant UUID"
// @Param Idempotency-Key header string false "Reuses the audit identity of a retried request"
// @Success 200 {object} apiresponse.SuccessResponse
// @Failure 400,401,403,404 {object} apiresponse.ProblemDetails
// @Router /api/v1/admin/variants/{variant_id} [delete]
func archiveProductVariantV1(f *adminApp.CatalogAdminFacade, renderer *apiresponse.ErrorRenderer) gin.HandlerFunc {
	return func(c *gin.Context) {
		cmd, ok := contentCommand(c, renderer)
		if !ok {
			return
		}
		variantID, ok := contentID(c, renderer, "variant_id")
		if !ok {
			return
		}
		variant, err := f.ArchiveVariant(c.Request.Context(), cmd, variantID)
		if err != nil {
			renderer.Abort(c, variantError(err))
			return
		}
		apiresponse.Success(c, http.StatusOK, variant)
	}
}

func variantError(err error) error {
	switch {
	case stderrors.Is(err, catalogDomain.ErrProductNotFound):
		return apiresponse.NotFound(err, "The product variant was not found.")
	case stderrors.Is(err, catalogDomain.ErrInvalidProduct):
		return apiresponse.ValidationFailed(err)
	default:
		return err
	}
}
