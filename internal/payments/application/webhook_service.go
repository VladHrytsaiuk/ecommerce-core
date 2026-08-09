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
	workflow paymentWorkflow
}

type paymentWorkflow interface {
	MarkPaid(context.Context, workflowDomain.PaymentConfirmation) error
	MarkFailed(context.Context, workflowDomain.PaymentConfirmation) error
}

func NewWebhookService(gateways *Registry, events paymentsDomain.WebhookEventStore, workflow paymentWorkflow) *WebhookService {
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
	if event.Provider != "" && event.Provider != provider {
		return fmt.Errorf("verified payment event provider %q does not match route provider %q", event.Provider, provider)
	}
	event.Provider = provider
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
	confirmation := workflowDomain.PaymentConfirmation{PaymentAttempt: workflowDomain.PaymentAttempt{OrderID: event.OrderID, Provider: event.Provider, ProviderReference: event.ProviderReference, Amount: event.Amount}, Status: event.Status}
	switch event.Status {
	case "paid":
		return s.workflow.MarkPaid(ctx, confirmation)
	case "failed", "cancelled", "expired":
		// All non-success terminal provider outcomes release the same local
		// reservation set, including an optional promo redemption.
		confirmation.Status = "failed"
		return s.workflow.MarkFailed(ctx, confirmation)
	case "pending":
		return nil
	default:
		return fmt.Errorf("%w: %q", paymentsDomain.ErrUnsupportedEvent, event.Status)
	}
}
