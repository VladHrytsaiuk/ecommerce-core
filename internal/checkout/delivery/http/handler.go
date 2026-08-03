// Package http exposes the clean checkout start-payment endpoint.
package http

import (
	stdhttp "net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	checkoutDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/checkout/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware"
)

type Handler struct {
	service        checkoutDomain.Service
	reservationTTL time.Duration
}

func NewHandler(service checkoutDomain.Service, reservationTTL time.Duration) *Handler {
	return &Handler{service: service, reservationTTL: reservationTTL}
}

type StartPaymentRequest struct {
	CustomerID       *uuid.UUID       `json:"customer_id,omitempty"`
	CustomerPhone    string           `json:"customer_phone,omitempty"`
	DeliveryProvider string           `json:"delivery_provider,omitempty"`
	Delivery         *DeliveryDetails `json:"delivery,omitempty"`
	ReturnURL        string           `json:"return_url,omitempty"`
	CancelURL        string           `json:"cancel_url,omitempty"`
	Lines            []Line           `json:"lines" binding:"required,min=1"`
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

type Line struct {
	VariantID   uuid.UUID `json:"variant_id" binding:"required"`
	WarehouseID uuid.UUID `json:"warehouse_id" binding:"required"`
	Quantity    int       `json:"quantity" binding:"required,gt=0"`
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

func (h *Handler) StartPayment(c *gin.Context) {
	var request StartPaymentRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(stdhttp.StatusBadRequest, gin.H{"error": "invalid checkout request", "message": err.Error()})
		return
	}
	lines := make([]checkoutDomain.Line, 0, len(request.Lines))
	for _, line := range request.Lines {
		lines = append(lines, checkoutDomain.Line{VariantID: line.VariantID, WarehouseID: line.WarehouseID, Quantity: line.Quantity})
	}
	started, err := h.service.StartPayment(c.Request.Context(), checkoutDomain.StartPaymentRequest{
		Preparation: checkoutDomain.PrepareRequest{CheckoutID: uuid.New(), Locale: middleware.GetLanguage(c), Lines: lines, ExpiresAt: time.Now().UTC().Add(h.reservationTTL)},
		CustomerID:  request.CustomerID, CustomerPhone: request.CustomerPhone, DeliveryProvider: request.DeliveryProvider, Delivery: mapDelivery(request.Delivery), ReturnURL: request.ReturnURL, CancelURL: request.CancelURL,
	})
	if err != nil {
		c.JSON(stdhttp.StatusUnprocessableEntity, gin.H{"error": "checkout could not be started", "message": err.Error()})
		return
	}
	c.JSON(stdhttp.StatusCreated, StartPaymentResponse{OrderID: started.Order.ID, OrderNumber: started.Order.Number, PaymentProvider: started.Order.PaymentProvider, ProviderReference: started.Session.ProviderReference, RedirectURL: started.Session.RedirectURL, PaymentForm: started.Session.FormFields, ClientSecret: started.Session.ClientSecret, ExpiresAt: started.Prepared.ExpiresAt})
}

func mapDelivery(details *DeliveryDetails) *checkoutDomain.DeliveryDetails {
	if details == nil {
		return nil
	}
	return &checkoutDomain.DeliveryDetails{RecipientName: details.RecipientName, RecipientPhone: details.RecipientPhone, CountryCode: details.CountryCode, PostalCode: details.PostalCode, City: details.City, Line1: details.Line1, Line2: details.Line2, LocalityID: details.LocalityID, ServicePointID: details.ServicePointID}
}
