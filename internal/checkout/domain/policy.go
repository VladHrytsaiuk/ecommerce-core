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
