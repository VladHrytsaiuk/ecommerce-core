package domain

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// Policy carries only checkout rules; it deliberately does not expose the
// global StoreConfig to application use cases.
type Policy struct {
	AllowGuest        bool
	RequirePhone      bool
	OrderNumberPrefix string
	// SupportedDeliveryProviders is assembled from the enabled adapter codes in
	// Bootstrap. Checkout stores only the selected code, never an adapter or a
	// provider-specific delivery field.
	SupportedDeliveryProviders []string
	DefaultDeliveryProvider    string
}

func (p Policy) OrderNumber(checkoutID uuid.UUID) string {
	prefix := strings.ToUpper(strings.TrimSpace(p.OrderNumberPrefix))
	if prefix == "" {
		prefix = "ORDER"
	}
	return prefix + "-" + strings.ToUpper(checkoutID.String()[:8])
}

func (p Policy) ValidateCustomer(customerID *uuid.UUID, phone string) error {
	if !p.AllowGuest && customerID == nil {
		return fmt.Errorf("guest checkout is disabled")
	}
	if p.RequirePhone && strings.TrimSpace(phone) == "" {
		return fmt.Errorf("customer phone is required")
	}
	return nil
}

// ResolveDeliveryProvider applies the store delivery policy before stock is
// reserved. This avoids persisting an arbitrary browser-supplied provider code
// on an order, while allowing a provider-free deployment to omit delivery.
func (p Policy) ResolveDeliveryProvider(requested string) (string, error) {
	requested = strings.ToLower(strings.TrimSpace(requested))
	if requested == "" {
		requested = strings.ToLower(strings.TrimSpace(p.DefaultDeliveryProvider))
	}
	if requested == "" {
		return "", nil
	}
	for _, provider := range p.SupportedDeliveryProviders {
		if strings.ToLower(strings.TrimSpace(provider)) == requested {
			return requested, nil
		}
	}
	return "", fmt.Errorf("delivery provider %q is not enabled", requested)
}
