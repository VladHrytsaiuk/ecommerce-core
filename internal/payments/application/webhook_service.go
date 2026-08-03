package application

import (
	"context"
	"fmt"
	"strings"

	workflowDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/core/orderworkflow/domain"
	paymentsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/payments/domain"
)

// WebhookService owns the provider-neutral payment transition. Verification
// remains in the adapter; order state remains in the atomic workflow.
type WebhookService struct {
	gateways *Registry
	events   paymentsDomain.WebhookEventStore
	workflow workflowDomain.Service
}

func NewWebhookService(gateways *Registry, events paymentsDomain.WebhookEventStore, workflow workflowDomain.Service) *WebhookService {
	return &WebhookService{gateways: gateways, events: events, workflow: workflow}
}

func (s *WebhookService) Handle(ctx context.Context, provider string, request paymentsDomain.WebhookRequest) error {
	if s.gateways == nil || s.events == nil || s.workflow == nil {
		return fmt.Errorf("payment webhook service is not configured")
	}
	provider = strings.ToLower(strings.TrimSpace(provider))
	gateway, enabled := s.gateways.Get(provider)
	if !enabled {
		return fmt.Errorf("%w: %s", paymentsDomain.ErrGatewayNotEnabled, provider)
	}
	event, err := gateway.VerifyWebhook(ctx, request)
	if err != nil {
		return err
	}
	claimed, err := s.events.Claim(ctx, provider, event)
	if err != nil || !claimed {
		return err
	}
	if err := s.apply(ctx, event); err != nil {
		_ = s.events.Abandon(context.WithoutCancel(ctx), provider, event.EventID)
		return err
	}
	if err := s.events.MarkProcessed(ctx, provider, event.EventID); err != nil {
		return err
	}
	return nil
}

func (s *WebhookService) apply(ctx context.Context, event paymentsDomain.PaymentEvent) error {
	switch event.Status {
	case "paid":
		return s.workflow.MarkPaid(ctx, event.OrderID)
	case "failed":
		return s.workflow.CancelPending(ctx, event.OrderID)
	case "pending":
		return nil
	default:
		return fmt.Errorf("%w: %q", paymentsDomain.ErrUnsupportedEvent, event.Status)
	}
}
