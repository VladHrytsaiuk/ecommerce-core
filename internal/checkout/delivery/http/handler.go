// Package http exposes the clean checkout start-payment endpoint.
package http

import (
	"fmt"
	stdhttp "net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	cartDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/cart/domain"
	checkoutDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/checkout/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/cartowner"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware"
)

type Handler struct {
	service        checkoutDomain.Service
	carts          cartDomain.Service
	reservationTTL time.Duration
	warehouseID    uuid.UUID
}

func NewHandler(service checkoutDomain.Service, carts cartDomain.Service, reservationTTL time.Duration, warehouseID uuid.UUID) *Handler {
	return &Handler{service: service, carts: carts, reservationTTL: reservationTTL, warehouseID: warehouseID}
}

type StartPaymentRequest struct {
	CustomerPhone      string           `json:"customer_phone,omitempty"`
	DeliveryProvider   string           `json:"delivery_provider,omitempty"`
	DeliveryOptionCode string           `json:"delivery_option_code,omitempty"`
	Delivery           *DeliveryDetails `json:"delivery,omitempty"`
	ReturnURL          string           `json:"return_url,omitempty"`
	CancelURL          string           `json:"cancel_url,omitempty"`
}

type DeliveryDetails struct {
	RecipientName  string `json:"recipient_name,omitempty"`
	RecipientPhone string `json:"recipient_phone,omitempty"`
	CountryCode    string `json:"country_code,omitempty"`
	PostalCode     string `json:"postal_code,omitempty"`
	City           string `json:"city,omitempty"`
	Line1          string `json:"line1,omitempty"`
	Line2          string `json:"line2,omitempty"`
	LocalityID     string `json:"locality_id,omitempty"`
	ServicePointID string `json:"service_point_id,omitempty"`
}

type StartPaymentResponse struct {
	OrderID           uuid.UUID         `json:"order_id"`
	OrderNumber       string            `json:"order_number"`
	PaymentProvider   string            `json:"payment_provider"`
	ProviderReference string            `json:"provider_reference"`
	RedirectURL       string            `json:"redirect_url"`
	PaymentForm       map[string]string `json:"payment_form,omitempty"`
	ClientSecret      string            `json:"client_secret,omitempty"`
	ExpiresAt         time.Time         `json:"expires_at"`
}

type DeliveryQuoteResponse struct {
	Provider string `json:"provider"`
	Options  any    `json:"options"`
}

func (h *Handler) QuoteDelivery(c *gin.Context) {
	var request StartPaymentRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(stdhttp.StatusBadRequest, gin.H{"error": "invalid delivery quote request", "message": err.Error()})
		return
	}
	lines, ok := h.cartLines(c)
	if !ok {
		return
	}
	quote, err := h.service.QuoteDelivery(c.Request.Context(), checkoutDomain.DeliveryQuoteRequest{Locale: middleware.GetLanguage(c), Lines: lines, DeliveryProvider: request.DeliveryProvider, Delivery: *mapDeliveryOrEmpty(request.Delivery)})
	if err != nil {
		c.JSON(stdhttp.StatusUnprocessableEntity, gin.H{"error": "delivery could not be quoted", "message": err.Error()})
		return
	}
	c.JSON(stdhttp.StatusOK, DeliveryQuoteResponse{Provider: quote.Provider, Options: quote.Options})
}

func (h *Handler) StartPayment(c *gin.Context) {
	var request StartPaymentRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(stdhttp.StatusBadRequest, gin.H{"error": "invalid checkout request", "message": err.Error()})
		return
	}
	owner, cart, lines, ok := h.ownerCartAndLines(c)
	if !ok {
		return
	}
	checkoutID, err := checkoutIDFromRequest(c)
	if err != nil {
		c.JSON(stdhttp.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	started, err := h.service.StartPayment(c.Request.Context(), checkoutDomain.StartPaymentRequest{
		Preparation: checkoutDomain.PrepareRequest{CheckoutID: checkoutID, Locale: middleware.GetLanguage(c), Lines: lines, ExpiresAt: time.Now().UTC().Add(h.reservationTTL)},
		CartID:      cart.ID, CustomerID: owner.CustomerID, CustomerPhone: request.CustomerPhone, DeliveryProvider: request.DeliveryProvider, DeliveryOptionCode: request.DeliveryOptionCode, Delivery: mapDelivery(request.Delivery), ReturnURL: request.ReturnURL, CancelURL: request.CancelURL,
	})
	if err != nil {
		c.JSON(stdhttp.StatusUnprocessableEntity, gin.H{"error": "checkout could not be started", "message": err.Error()})
		return
	}
	c.JSON(stdhttp.StatusCreated, StartPaymentResponse{OrderID: started.Order.ID, OrderNumber: started.Order.Number, PaymentProvider: started.Order.PaymentProvider, ProviderReference: started.Session.ProviderReference, RedirectURL: started.Session.RedirectURL, PaymentForm: started.Session.FormFields, ClientSecret: started.Session.ClientSecret, ExpiresAt: started.Prepared.ExpiresAt})
}

// checkoutIDFromRequest makes one logical browser checkout stable across safe
// HTTP retries. It is intentionally derived from the opaque key alone: an
// anonymous caller can lose the first response before receiving a cart cookie.
// The client must generate a high-entropy key per checkout attempt and reuse
// it only while retrying that attempt.
func checkoutIDFromRequest(c *gin.Context) (uuid.UUID, error) {
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" || len(key) > 128 {
		return uuid.Nil, fmt.Errorf("Idempotency-Key header is required and must be at most 128 characters")
	}
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("ecommerce-core:checkout:"+key)), nil
}

func (h *Handler) cartLines(c *gin.Context) ([]checkoutDomain.Line, bool) {
	_, _, lines, ok := h.ownerCartAndLines(c)
	return lines, ok
}

func (h *Handler) ownerCartAndLines(c *gin.Context) (cartDomain.Owner, *cartDomain.Cart, []checkoutDomain.Line, bool) {
	owner, created, err := cartowner.FromContext(c)
	if err != nil {
		c.JSON(stdhttp.StatusBadRequest, gin.H{"error": err.Error()})
		return cartDomain.Owner{}, nil, nil, false
	}
	if created {
		cartowner.SetSessionCookie(c, *owner.SessionID)
	}
	cart, err := h.carts.GetOrCreate(c.Request.Context(), owner)
	if err != nil {
		c.JSON(stdhttp.StatusUnprocessableEntity, gin.H{"error": "cart could not be loaded", "message": err.Error()})
		return cartDomain.Owner{}, nil, nil, false
	}
	if len(cart.Items) == 0 {
		c.JSON(stdhttp.StatusUnprocessableEntity, gin.H{"error": "cart is empty"})
		return cartDomain.Owner{}, nil, nil, false
	}
	lines := make([]checkoutDomain.Line, 0, len(cart.Items))
	for _, item := range cart.Items {
		lines = append(lines, checkoutDomain.Line{VariantID: item.VariantID, WarehouseID: h.warehouseID, Quantity: item.Quantity})
	}
	return owner, cart, lines, true
}

func mapDelivery(details *DeliveryDetails) *checkoutDomain.DeliveryDetails {
	if details == nil {
		return nil
	}
	return &checkoutDomain.DeliveryDetails{RecipientName: details.RecipientName, RecipientPhone: details.RecipientPhone, CountryCode: details.CountryCode, PostalCode: details.PostalCode, City: details.City, Line1: details.Line1, Line2: details.Line2, LocalityID: details.LocalityID, ServicePointID: details.ServicePointID}
}

func mapDeliveryOrEmpty(details *DeliveryDetails) *checkoutDomain.DeliveryDetails {
	if mapped := mapDelivery(details); mapped != nil {
		return mapped
	}
	return &checkoutDomain.DeliveryDetails{}
}
