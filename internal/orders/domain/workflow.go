package domain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrWorkflowStatusDisabled    = errors.New("order workflow status is disabled")
	ErrWorkflowTransitionInvalid = errors.New("order workflow transition is invalid")
	ErrWorkflowTriggerDenied     = errors.New("order workflow trigger is not allowed")
	ErrWorkflowRequirement       = errors.New("order workflow transition requirement is not satisfied")
	ErrWorkflowNotFound          = errors.New("order workflow transition is not configured")
	ErrWorkflowSystemManaged     = errors.New("system-managed workflow entry cannot be changed")
)

// StatusKind classifies a workflow status.  Financial status effects remain
// enforced by the OrderWorkflow application service; this classification is
// intentionally not permission to bypass those invariants.
type StatusKind string

const (
	StatusKindPayment     StatusKind = "payment"
	StatusKindFulfillment StatusKind = "fulfillment"
	StatusKindTerminal    StatusKind = "terminal"
	StatusKindCustom      StatusKind = "custom"
)

// TransitionTrigger identifies the trusted source requesting a status change.
// Webhook triggers are assigned only after provider-signature verification.
type TransitionTrigger string

const (
	TransitionTriggerAdmin           TransitionTrigger = "admin"
	TransitionTriggerSystem          TransitionTrigger = "system"
	TransitionTriggerPaymentWebhook  TransitionTrigger = "payment_webhook"
	TransitionTriggerDeliveryWebhook TransitionTrigger = "delivery_webhook"
	TransitionTriggerCustomer        TransitionTrigger = "customer"
)

// StatusActorType identifies the actor recorded in immutable history.
type StatusActorType string

const (
	StatusActorAdmin           StatusActorType = "admin"
	StatusActorSystem          StatusActorType = "system"
	StatusActorPaymentWebhook  StatusActorType = "payment_webhook"
	StatusActorDeliveryWebhook StatusActorType = "delivery_webhook"
	StatusActorCustomer        StatusActorType = "customer"
)

// OrderStatusDefinition is an administrator-configurable operational status.
// System-managed entries may be read but cannot later be altered through an
// admin workflow API.
type OrderStatusDefinition struct {
	Code          string     `json:"code"`
	Name          string     `json:"name"`
	Description   string     `json:"description"`
	Color         string     `json:"color"`
	SortOrder     int        `json:"sort_order"`
	Kind          StatusKind `json:"kind"`
	IsInitial     bool       `json:"is_initial"`
	IsTerminal    bool       `json:"is_terminal"`
	SystemManaged bool       `json:"system_managed"`
	CustomerLabel string     `json:"customer_label"`
	Enabled       bool       `json:"enabled"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// OrderStatusTransition defines one permitted directed transition.  Required
// Permission is evaluated only for an admin trigger; provider and system
// paths still require their trusted authentication and core invariants.
type OrderStatusTransition struct {
	ID                     uuid.UUID           `json:"id"`
	FromStatusCode         string              `json:"from_status_code"`
	ToStatusCode           string              `json:"to_status_code"`
	AllowedTriggers        []TransitionTrigger `json:"allowed_triggers"`
	RequiresPayment        bool                `json:"requires_payment"`
	RequiresTrackingNumber bool                `json:"requires_tracking_number"`
	RequiresReason         bool                `json:"requires_reason"`
	RequiredPermission     string              `json:"required_permission"`
	CreatedAt              time.Time           `json:"created_at"`
	UpdatedAt              time.Time           `json:"updated_at"`
}

// OrderStatusHistory is append-only evidence of an attempted and committed
// state change. EventID makes asynchronous webhook transitions idempotent.
type OrderStatusHistory struct {
	ID             uuid.UUID       `json:"id"`
	OrderID        uuid.UUID       `json:"order_id"`
	FromStatusCode string          `json:"from_status_code,omitempty"`
	ToStatusCode   string          `json:"to_status_code"`
	ActorType      StatusActorType `json:"actor_type"`
	ActorID        *uuid.UUID      `json:"actor_id,omitempty"`
	Reason         string          `json:"reason,omitempty"`
	Metadata       json.RawMessage `json:"metadata"`
	EventID        *uuid.UUID      `json:"event_id,omitempty"`
	OccurredAt     time.Time       `json:"occurred_at"`
	CreatedAt      time.Time       `json:"created_at"`
}

// TransitionPolicy is the minimum configuration required to make a
// transaction-safe transition decision. Repositories must load all three rows
// from the same database context; callers pass a transaction context whenever
// the decision is paired with an order mutation.
type TransitionPolicy struct {
	From       OrderStatusDefinition
	To         OrderStatusDefinition
	Transition OrderStatusTransition
}

// WorkflowRepository owns only workflow configuration and its append-only
// history. It never exposes or mutates another module's order persistence.
type WorkflowRepository interface {
	LoadTransitionPolicy(context.Context, string, string) (TransitionPolicy, error)
	AppendStatusHistory(context.Context, OrderStatusHistory) (inserted bool, err error)
}

// WorkflowConfigurationRepository is the administration port. Disabling a
// definition is preferred to deletion so historic status records remain
// meaningful forever.
type WorkflowConfigurationRepository interface {
	ListWorkflowConfiguration(context.Context) ([]OrderStatusDefinition, []OrderStatusTransition, error)
	ListStatusHistory(context.Context, uuid.UUID) ([]OrderStatusHistory, error)
	SaveStatusDefinition(context.Context, OrderStatusDefinition) error
	SaveStatusTransition(context.Context, OrderStatusTransition) error
}

// TransitionRequest contains only facts already verified by its delivery
// adapter. In particular, payment and delivery webhooks must be authenticated
// before their trigger reaches this policy.
type TransitionRequest struct {
	FromStatusCode   string
	ToStatusCode     string
	Trigger          TransitionTrigger
	PaymentConfirmed bool
	TrackingNumber   string
	Reason           string
}

// ValidateTransition applies data-driven workflow policy while deliberately
// keeping financial side effects outside this package. The caller is still
// responsible for enforcing the returned RequiredPermission for admin actions
// and for atomically applying the status change plus history entry.
func ValidateTransition(from, to OrderStatusDefinition, transition OrderStatusTransition, request TransitionRequest) error {
	if !from.Enabled || !to.Enabled {
		return ErrWorkflowStatusDisabled
	}
	if strings.TrimSpace(request.FromStatusCode) == "" ||
		strings.TrimSpace(request.ToStatusCode) == "" ||
		request.FromStatusCode != from.Code || request.ToStatusCode != to.Code ||
		transition.FromStatusCode != from.Code || transition.ToStatusCode != to.Code ||
		from.Code == to.Code || to.IsInitial || from.IsTerminal {
		return ErrWorkflowTransitionInvalid
	}
	if !allowsTrigger(transition.AllowedTriggers, request.Trigger) {
		return ErrWorkflowTriggerDenied
	}
	if transition.RequiresPayment && !request.PaymentConfirmed {
		return fmt.Errorf("%w: confirmed payment is required", ErrWorkflowRequirement)
	}
	if transition.RequiresTrackingNumber && strings.TrimSpace(request.TrackingNumber) == "" {
		return fmt.Errorf("%w: tracking number is required", ErrWorkflowRequirement)
	}
	if transition.RequiresReason && strings.TrimSpace(request.Reason) == "" {
		return fmt.Errorf("%w: reason is required", ErrWorkflowRequirement)
	}
	return nil
}

func allowsTrigger(allowed []TransitionTrigger, requested TransitionTrigger) bool {
	for _, trigger := range allowed {
		if trigger == requested {
			return true
		}
	}
	return false
}
