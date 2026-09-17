package application

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/orders/domain"
)

var workflowCode = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// WorkflowService evaluates the data-driven operational state machine. It is
// intentionally side-effect free: the OrderWorkflow integration will use the
// returned policy inside the transaction that changes the actual order.
type WorkflowService struct {
	repository domain.WorkflowRepository
}

func (s *WorkflowService) ListConfiguration(ctx context.Context) ([]domain.OrderStatusDefinition, []domain.OrderStatusTransition, error) {
	repository, ok := s.repository.(domain.WorkflowConfigurationRepository)
	if !ok {
		return nil, nil, fmt.Errorf("order workflow configuration is not supported")
	}
	return repository.ListWorkflowConfiguration(ctx)
}

func (s *WorkflowService) ListStatusHistory(ctx context.Context, orderID uuid.UUID) ([]domain.OrderStatusHistory, error) {
	if orderID == uuid.Nil {
		return nil, fmt.Errorf("invalid order id")
	}
	repository, ok := s.repository.(domain.WorkflowConfigurationRepository)
	if !ok {
		return nil, fmt.Errorf("order workflow configuration is not supported")
	}
	return repository.ListStatusHistory(ctx, orderID)
}

func (s *WorkflowService) SaveStatusDefinition(ctx context.Context, definition domain.OrderStatusDefinition) error {
	repository, ok := s.repository.(domain.WorkflowConfigurationRepository)
	if !ok {
		return fmt.Errorf("order workflow configuration is not supported")
	}
	definition.Code = strings.TrimSpace(strings.ToLower(definition.Code))
	definition.Name = strings.TrimSpace(definition.Name)
	if !workflowCode.MatchString(definition.Code) || definition.Name == "" || !validStatusKind(definition.Kind) {
		return fmt.Errorf("invalid order status definition")
	}
	// Payment states are an intentionally closed set owned by the dedicated
	// checkout and payment-webhook workflow. Allowing an administrator to add a
	// new, apparently operational payment state would create a route around its
	// inventory, payment-provider and refund invariants. The migration seeds the
	// system-managed payment states; the administration API owns only
	// fulfillment, terminal and custom definitions.
	if definition.Kind == domain.StatusKindPayment {
		return fmt.Errorf("payment status definitions are system-managed")
	}
	// System ownership is established only by migrations, never a client DTO.
	definition.SystemManaged = false
	return repository.SaveStatusDefinition(ctx, definition)
}

func (s *WorkflowService) SaveStatusTransition(ctx context.Context, transition domain.OrderStatusTransition) error {
	repository, ok := s.repository.(domain.WorkflowConfigurationRepository)
	if !ok {
		return fmt.Errorf("order workflow configuration is not supported")
	}
	transition.FromStatusCode = strings.TrimSpace(strings.ToLower(transition.FromStatusCode))
	transition.ToStatusCode = strings.TrimSpace(strings.ToLower(transition.ToStatusCode))
	if !workflowCode.MatchString(transition.FromStatusCode) || !workflowCode.MatchString(transition.ToStatusCode) || transition.FromStatusCode == transition.ToStatusCode || len(transition.AllowedTriggers) == 0 {
		return fmt.Errorf("invalid order status transition")
	}
	for _, trigger := range transition.AllowedTriggers {
		if !validTrigger(trigger) {
			return fmt.Errorf("invalid order status transition trigger")
		}
	}
	return repository.SaveStatusTransition(ctx, transition)
}

func validStatusKind(kind domain.StatusKind) bool {
	return kind == domain.StatusKindPayment || kind == domain.StatusKindFulfillment || kind == domain.StatusKindTerminal || kind == domain.StatusKindCustom
}

func validTrigger(trigger domain.TransitionTrigger) bool {
	switch trigger {
	case domain.TransitionTriggerAdmin, domain.TransitionTriggerSystem, domain.TransitionTriggerPaymentWebhook, domain.TransitionTriggerDeliveryWebhook, domain.TransitionTriggerCustomer:
		return true
	default:
		return false
	}
}

func NewWorkflowService(repository domain.WorkflowRepository) (*WorkflowService, error) {
	if repository == nil {
		return nil, fmt.Errorf("order workflow repository is required")
	}
	return &WorkflowService{repository: repository}, nil
}

// ValidateTransition loads the configured edge and validates all policy facts.
// An admin caller must enforce the returned RequiredPermission through RBAC;
// webhook callers must already have verified their provider signature.
func (s *WorkflowService) ValidateTransition(ctx context.Context, request domain.TransitionRequest) (requiredPermission string, err error) {
	if strings.TrimSpace(request.FromStatusCode) == "" || strings.TrimSpace(request.ToStatusCode) == "" {
		return "", domain.ErrWorkflowTransitionInvalid
	}
	policy, err := s.repository.LoadTransitionPolicy(ctx, request.FromStatusCode, request.ToStatusCode)
	if err != nil {
		return "", err
	}
	if err := domain.ValidateTransition(policy.From, policy.To, policy.Transition, request); err != nil {
		return "", err
	}
	return policy.Transition.RequiredPermission, nil
}

// AppendStatusHistory exposes idempotent recording for the transaction owner.
// It must be invoked only after the corresponding order status update has
// succeeded in the same context-bound SQL transaction.
func (s *WorkflowService) AppendStatusHistory(ctx context.Context, history domain.OrderStatusHistory) (bool, error) {
	if history.OrderID == [16]byte{} || strings.TrimSpace(history.ToStatusCode) == "" || strings.TrimSpace(string(history.ActorType)) == "" {
		return false, fmt.Errorf("invalid order status history")
	}
	return s.repository.AppendStatusHistory(ctx, history)
}
