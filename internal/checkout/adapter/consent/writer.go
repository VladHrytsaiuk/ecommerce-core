// Package consent adapts the optional Consent module to Checkout's narrow
// marketing-consent port. Checkout never imports Consent's repository.
package consent

import (
	"context"

	"github.com/google/uuid"

	checkout "github.com/VladHrytsaiuk/ecommerce-core/internal/checkout/domain"
	consentApp "github.com/VladHrytsaiuk/ecommerce-core/internal/consent/application"
)

type Writer struct{ service *consentApp.Service }

func NewWriter(service *consentApp.Service) *Writer { return &Writer{service: service} }

func (w *Writer) GrantActiveMarketing(ctx context.Context, customerID *uuid.UUID, email, ip string) error {
	return w.service.GrantActiveMarketing(ctx, customerID, email, ip)
}

var _ checkout.MarketingConsentWriter = (*Writer)(nil)
