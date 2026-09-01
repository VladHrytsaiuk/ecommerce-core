package http

import (
	"errors"
	"fmt"
	stdhttp "net/http"
	"time"

	"github.com/gin-gonic/gin"

	cartDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/cart/domain"
	checkoutDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/checkout/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/apiresponse"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/cartowner"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware"
)

// CheckoutV1Handler is the RFC 9457/API-envelope transport facade over the
// existing checkout application service.
type CheckoutV1Handler struct {
	legacy *Handler
	errors *apiresponse.ErrorRenderer
}

func NewCheckoutV1Handler(legacy *Handler, renderer *apiresponse.ErrorRenderer) *CheckoutV1Handler {
	return &CheckoutV1Handler{legacy: legacy, errors: renderer}
}

// QuoteDelivery godoc
// @Summary Quote delivery options (v1)
// @Tags Checkout v1
// @Accept json
// @Produce json
// @Param lang path string true "Locale"
// @Param request body StartPaymentRequest true "Checkout delivery request"
// @Success 200 {object} apiresponse.SuccessResponse
// @Failure 400 {object} apiresponse.ProblemDetails
// @Failure 422 {object} apiresponse.ProblemDetails
// @Router /api/v1/checkout/{lang}/delivery-options [post]
func (h *CheckoutV1Handler) QuoteDelivery(c *gin.Context) {
	var request StartPaymentRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.errors.Abort(c, apiresponse.InvalidPayload(err))
		return
	}
	_, _, lines, err := h.ownerCartAndLines(c)
	if err != nil {
		h.errors.Abort(c, checkoutValidationError(err))
		return
	}
	quote, err := h.legacy.service.QuoteDelivery(c.Request.Context(), checkoutDomain.DeliveryQuoteRequest{Locale: middleware.GetLanguage(c), Lines: lines, DeliveryProvider: request.DeliveryProvider, Delivery: *mapDeliveryOrEmpty(request.Delivery)})
	if err != nil {
		h.errors.Abort(c, apiresponse.ValidationFailed(err))
		return
	}
	apiresponse.Success(c, stdhttp.StatusOK, DeliveryQuoteResponse{Provider: quote.Provider, Options: quote.Options})
}

func checkoutValidationError(err error) *apiresponse.PublicError {
	var incomplete *checkoutDomain.ProfileIncompleteError
	if errors.As(err, &incomplete) {
		invalid := make([]apiresponse.InvalidParam, 0, len(incomplete.MissingFields))
		for _, field := range incomplete.MissingFields {
			invalid = append(invalid, apiresponse.InvalidParam{Field: field, Code: "required"})
		}
		return apiresponse.ValidationFailed(err, invalid...)
	}
	return apiresponse.ValidationFailed(err)
}

// StartPayment godoc
// @Summary Start an idempotent checkout payment (v1)
// @Tags Checkout v1
// @Accept json
// @Produce json
// @Param lang path string true "Locale"
// @Param Idempotency-Key header string true "Stable high-entropy checkout key"
// @Param request body StartPaymentRequest true "Checkout payment request"
// @Success 201 {object} apiresponse.SuccessResponse
// @Failure 400 {object} apiresponse.ProblemDetails
// @Failure 422 {object} apiresponse.ProblemDetails
// @Router /api/v1/checkout/{lang}/payment [post]
func (h *CheckoutV1Handler) StartPayment(c *gin.Context) {
	var request StartPaymentRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.errors.Abort(c, apiresponse.InvalidPayload(err))
		return
	}
	owner, cart, lines, err := h.ownerCartAndLines(c)
	if err != nil {
		h.errors.Abort(c, apiresponse.ValidationFailed(err))
		return
	}
	checkoutID, err := checkoutIDFromRequest(c)
	if err != nil {
		h.errors.Abort(c, apiresponse.InvalidPayload(err))
		return
	}
	started, err := h.legacy.service.StartPayment(c.Request.Context(), checkoutDomain.StartPaymentRequest{
		Preparation: checkoutDomain.PrepareRequest{CheckoutID: checkoutID, Locale: middleware.GetLanguage(c), Lines: lines, ExpiresAt: time.Now().UTC().Add(h.legacy.reservationTTL), PromoCode: cart.AppliedPromoCode},
		CartID:      cart.ID, CustomerID: owner.CustomerID, CustomerEmail: request.CustomerEmail, CustomerPhone: request.CustomerPhone, DeliveryProvider: request.DeliveryProvider, DeliveryOptionCode: request.DeliveryOptionCode, Delivery: mapDelivery(request.Delivery), ReturnURL: request.ReturnURL, CancelURL: request.CancelURL,
	})
	if err != nil {
		h.errors.Abort(c, checkoutValidationError(err))
		return
	}
	apiresponse.Success(c, stdhttp.StatusCreated, StartPaymentResponse{OrderID: started.Order.ID, OrderNumber: started.Order.Number, PaymentProvider: started.Order.PaymentProvider, ProviderReference: started.Session.ProviderReference, RedirectURL: started.Session.RedirectURL, PaymentForm: started.Session.FormFields, ClientSecret: started.Session.ClientSecret, ExpiresAt: started.Prepared.ExpiresAt})
}

func (h *CheckoutV1Handler) ownerCartAndLines(c *gin.Context) (cartDomain.Owner, *cartDomain.Cart, []checkoutDomain.Line, error) {
	owner, created, err := cartowner.FromContext(c)
	if err != nil {
		return cartDomain.Owner{}, nil, nil, err
	}
	if created {
		cartowner.SetSessionCookie(c, *owner.SessionID, h.legacy.secureCookies)
	}
	cart, err := h.legacy.carts.GetOrCreate(c.Request.Context(), owner)
	if err != nil {
		return cartDomain.Owner{}, nil, nil, fmt.Errorf("load cart: %w", err)
	}
	if len(cart.Items) == 0 {
		return cartDomain.Owner{}, nil, nil, fmt.Errorf("cart is empty")
	}
	lines := make([]checkoutDomain.Line, 0, len(cart.Items))
	for _, item := range cart.Items {
		lines = append(lines, checkoutDomain.Line{VariantID: item.VariantID, WarehouseID: h.legacy.warehouseID, Quantity: item.Quantity})
	}
	return owner, cart, lines, nil
}
