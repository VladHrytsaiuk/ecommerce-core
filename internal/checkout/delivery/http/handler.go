// Package http exposes the clean checkout start-payment endpoint.
package http

import (
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	cartDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/cart/domain"
	checkoutDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/checkout/domain"
)

type Handler struct {
	service        checkoutDomain.Service
	carts          cartDomain.Service
	reservationTTL time.Duration
	warehouseID    uuid.UUID
	secureCookies  bool
}

func NewHandler(service checkoutDomain.Service, carts cartDomain.Service, reservationTTL time.Duration, warehouseID uuid.UUID, secureCookies bool) *Handler {
	return &Handler{service: service, carts: carts, reservationTTL: reservationTTL, warehouseID: warehouseID, secureCookies: secureCookies}
}

type StartPaymentRequest struct {
	CustomerEmail      string           `json:"customer_email"`
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

// Handler holds what the v1 handler needs: the checkout service, the cart
// service, the reservation window, the default warehouse and the cookie flag.
//
// It used to serve requests itself, on a second registration alongside v1.
// Those methods wrote err.Error() into the response body, so an unexpected
// failure reached the buyer carrying PostgreSQL's text — and as a 422, which
// tells a client the request was invalid and not to retry. The registration
// and the methods are gone; v1 renders a classified problem document and logs
// the rest.

// checkoutIDFromRequest derives a stable checkout id from the caller's
// idempotency key, so a retried payment start reuses the same reservations
// rather than taking a second set.
func checkoutIDFromRequest(c *gin.Context) (uuid.UUID, error) {
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" || len(key) > 128 {
		return uuid.Nil, fmt.Errorf("Idempotency-Key header is required and must be at most 128 characters")
	}
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("ecommerce-core:checkout:"+key)), nil
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
