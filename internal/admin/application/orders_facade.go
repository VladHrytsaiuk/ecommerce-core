package application

import (
	"context"
	"encoding/json"
	"fmt"
	adminDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
	workflowDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/core/orderworkflow/domain"
	ordersDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/orders/domain"
	"github.com/google/uuid"
)

const PermissionOrdersWrite = "orders:write"

type PendingOrderCanceller interface {
	CancelPendingWithActor(context.Context, workflowDomain.AdminCancellation) error
}

type OperationalOrderWorkflow interface {
	CurrentStatusForUpdate(context.Context, uuid.UUID) (string, error)
	TransitionOperational(context.Context, workflowDomain.OperationalStatusTransition) error
}

type OrderStatusPolicy interface {
	ValidateTransition(context.Context, ordersDomain.TransitionRequest) (string, error)
}

type OrderWorkflowConfiguration interface {
	ListConfiguration(context.Context) ([]ordersDomain.OrderStatusDefinition, []ordersDomain.OrderStatusTransition, error)
	ListStatusHistory(context.Context, uuid.UUID) ([]ordersDomain.OrderStatusHistory, error)
	SaveStatusDefinition(context.Context, ordersDomain.OrderStatusDefinition) error
	SaveStatusTransition(context.Context, ordersDomain.OrderStatusTransition) error
}

type OrdersAdminFacade struct {
	authorizer     adminDomain.Authorizer
	workflow       PendingOrderCanceller
	tx             TransactionManager
	publisher      events.TransactionalEventPublisher
	statusWorkflow OperationalOrderWorkflow
	statusPolicy   OrderStatusPolicy
	workflowConfig OrderWorkflowConfiguration
}

func (f *OrdersAdminFacade) WithWorkflowConfiguration(configuration OrderWorkflowConfiguration) *OrdersAdminFacade {
	if f != nil && configuration != nil {
		f.workflowConfig = configuration
	}
	return f
}

// WithStatusWorkflow attaches the data-driven operational state machine. It
// is optional during the additive rollout so legacy pending-payment cancel
// semantics remain available before the orders module is enabled.
func (f *OrdersAdminFacade) WithStatusWorkflow(policy OrderStatusPolicy, workflow OperationalOrderWorkflow) *OrdersAdminFacade {
	if f != nil && policy != nil && workflow != nil {
		f.statusPolicy = policy
		f.statusWorkflow = workflow
	}
	return f
}

func (f *OrdersAdminFacade) HasStatusWorkflow() bool {
	return f != nil && f.statusPolicy != nil && f.statusWorkflow != nil && f.workflowConfig != nil
}

func NewOrdersAdminFacade(a adminDomain.Authorizer, w PendingOrderCanceller, tx TransactionManager, p events.TransactionalEventPublisher) (*OrdersAdminFacade, error) {
	if a == nil || w == nil || tx == nil || p == nil {
		return nil, fmt.Errorf("orders admin facade is not configured")
	}
	return &OrdersAdminFacade{authorizer: a, workflow: w, tx: tx, publisher: p}, nil
}

func (f *OrdersAdminFacade) ListWorkflowConfiguration(ctx context.Context, actor uuid.UUID) ([]ordersDomain.OrderStatusDefinition, []ordersDomain.OrderStatusTransition, error) {
	if f.workflowConfig == nil {
		return nil, nil, fmt.Errorf("order workflow module is not configured")
	}
	if err := f.authorizer.Require(ctx, actor, "orders:workflow:read"); err != nil {
		return nil, nil, err
	}
	return f.workflowConfig.ListConfiguration(ctx)
}

// ListOrderStatusHistory returns the immutable lifecycle journal. It is
// deliberately a workflow-read permission rather than a broad order-read
// permission because metadata can include operational evidence such as a
// carrier tracking reference.
func (f *OrdersAdminFacade) ListOrderStatusHistory(ctx context.Context, actor, orderID uuid.UUID) ([]ordersDomain.OrderStatusHistory, error) {
	if f.workflowConfig == nil {
		return nil, fmt.Errorf("order workflow module is not configured")
	}
	if orderID == uuid.Nil {
		return nil, fmt.Errorf("invalid order id")
	}
	if err := f.authorizer.Require(ctx, actor, "orders:workflow:read"); err != nil {
		return nil, err
	}
	return f.workflowConfig.ListStatusHistory(ctx, orderID)
}

type OrderStatusDefinitionCommand struct {
	ActorUserID uuid.UUID
	EventKey    uuid.UUID
	IPAddress   string
	Definition  ordersDomain.OrderStatusDefinition
}

func (f *OrdersAdminFacade) SaveStatusDefinition(ctx context.Context, command OrderStatusDefinitionCommand) error {
	if f.workflowConfig == nil {
		return fmt.Errorf("order workflow module is not configured")
	}
	if err := f.authorizer.Require(ctx, command.ActorUserID, "orders:workflow:write"); err != nil {
		return err
	}
	if command.EventKey == uuid.Nil {
		command.EventKey = uuid.New()
	}
	return f.tx.WithinTransaction(ctx, func(tx context.Context) error {
		if err := f.workflowConfig.SaveStatusDefinition(tx, command.Definition); err != nil {
			return err
		}
		payload, err := json.Marshal(command.Definition)
		if err != nil {
			return err
		}
		event, err := adminDomain.NewAdminActionEvent(command.EventKey, command.ActorUserID, "orders.workflow.status.save", "order_status", uuid.NewSHA1(uuid.Nil, []byte(command.Definition.Code)), nil, payload, command.IPAddress, nil, nowUTC())
		if err != nil {
			return err
		}
		return f.publisher.Publish(tx, event)
	})
}

type OrderStatusTransitionCommand struct {
	ActorUserID uuid.UUID
	EventKey    uuid.UUID
	IPAddress   string
	Transition  ordersDomain.OrderStatusTransition
}

func (f *OrdersAdminFacade) SaveStatusTransition(ctx context.Context, command OrderStatusTransitionCommand) error {
	if f.workflowConfig == nil {
		return fmt.Errorf("order workflow module is not configured")
	}
	if err := f.authorizer.Require(ctx, command.ActorUserID, "orders:workflow:write"); err != nil {
		return err
	}
	if command.EventKey == uuid.Nil {
		command.EventKey = uuid.New()
	}
	return f.tx.WithinTransaction(ctx, func(tx context.Context) error {
		if err := f.workflowConfig.SaveStatusTransition(tx, command.Transition); err != nil {
			return err
		}
		payload, err := json.Marshal(command.Transition)
		if err != nil {
			return err
		}
		resource := command.Transition.FromStatusCode + "->" + command.Transition.ToStatusCode
		event, err := adminDomain.NewAdminActionEvent(command.EventKey, command.ActorUserID, "orders.workflow.transition.save", "order_status_transition", uuid.NewSHA1(uuid.Nil, []byte(resource)), nil, payload, command.IPAddress, nil, nowUTC())
		if err != nil {
			return err
		}
		return f.publisher.Publish(tx, event)
	})
}
func (f *OrdersAdminFacade) Cancel(ctx context.Context, actor, key, orderID uuid.UUID, ip, reason string) error {
	if err := f.authorizer.Require(ctx, actor, PermissionOrdersWrite); err != nil {
		return err
	}
	if key == uuid.Nil {
		key = uuid.New()
	}
	return f.tx.WithinTransaction(ctx, func(tx context.Context) error {
		if err := f.workflow.CancelPendingWithActor(tx, workflowDomain.AdminCancellation{OrderID: orderID, ActorID: actor, Reason: reason, EventID: key}); err != nil {
			return err
		}
		old, _ := json.Marshal(map[string]string{"status": "pending_payment"})
		next, _ := json.Marshal(map[string]string{"status": "cancelled"})
		metadata, _ := json.Marshal(map[string]string{"reason": reason})
		e, err := adminDomain.NewAdminActionEvent(key, actor, "orders.cancel", "order", orderID, old, next, ip, metadata, nowUTC())
		if err != nil {
			return err
		}
		return f.publisher.Publish(tx, e)
	})
}

type OrderTransitionCommand struct {
	ActorUserID    uuid.UUID
	EventKey       uuid.UUID
	IPAddress      string
	OrderID        uuid.UUID
	ToStatusCode   string
	Reason         string
	TrackingNumber string
}

// Transition applies a configured operational transition and appends both
// specialised order history and a generic admin audit event in one SQL unit
// of work. Financial transitions remain unavailable through this API.
func (f *OrdersAdminFacade) Transition(ctx context.Context, command OrderTransitionCommand) error {
	if f.statusPolicy == nil || f.statusWorkflow == nil {
		return fmt.Errorf("order workflow module is not configured")
	}
	if command.ActorUserID == uuid.Nil || command.OrderID == uuid.Nil || command.ToStatusCode == "" {
		return fmt.Errorf("invalid order status transition command")
	}
	if err := f.authorizer.Require(ctx, command.ActorUserID, PermissionOrdersWrite); err != nil {
		return err
	}
	if command.EventKey == uuid.Nil {
		command.EventKey = uuid.New()
	}
	return f.tx.WithinTransaction(ctx, func(tx context.Context) error {
		fromStatus, err := f.statusWorkflow.CurrentStatusForUpdate(tx, command.OrderID)
		if err != nil {
			return err
		}
		request := ordersDomain.TransitionRequest{
			FromStatusCode: fromStatus, ToStatusCode: command.ToStatusCode,
			Trigger: ordersDomain.TransitionTriggerAdmin, PaymentConfirmed: fromStatus != ordersDomain.StatusPendingPayment,
			TrackingNumber: command.TrackingNumber, Reason: command.Reason,
		}
		permission, err := f.statusPolicy.ValidateTransition(tx, request)
		if err != nil {
			return err
		}
		if permission != "" {
			if err := f.authorizer.Require(tx, command.ActorUserID, permission); err != nil {
				return err
			}
		}
		metadata, err := json.Marshal(map[string]string{"trigger": string(ordersDomain.TransitionTriggerAdmin), "tracking_number": command.TrackingNumber})
		if err != nil {
			return fmt.Errorf("marshal order status metadata: %w", err)
		}
		if err := f.statusWorkflow.TransitionOperational(tx, workflowDomain.OperationalStatusTransition{
			OrderID: command.OrderID, FromStatusCode: fromStatus, ToStatusCode: command.ToStatusCode,
			Trigger: ordersDomain.TransitionTriggerAdmin, PaymentConfirmed: request.PaymentConfirmed,
			TrackingNumber: command.TrackingNumber, ActorType: ordersDomain.StatusActorAdmin,
			ActorID: &command.ActorUserID, Reason: command.Reason, Metadata: metadata,
			EventID: command.EventKey, OccurredAt: nowUTC(),
		}); err != nil {
			return err
		}
		oldPayload, _ := json.Marshal(map[string]string{"status": fromStatus})
		newPayload, _ := json.Marshal(map[string]string{"status": command.ToStatusCode})
		event, err := adminDomain.NewAdminActionEvent(command.EventKey, command.ActorUserID, "orders.status.transition", "order", command.OrderID, oldPayload, newPayload, command.IPAddress, metadata, nowUTC())
		if err != nil {
			return err
		}
		return f.publisher.Publish(tx, event)
	})
}
