package application

import (
	"context"
	"testing"

	adminDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
	workflowDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/core/orderworkflow/domain"
	ordersDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/orders/domain"
	"github.com/google/uuid"
)

func TestOrdersAdminFacadeTransitionAuthorizesConfiguredPermissionAndAudits(t *testing.T) {
	actor, orderID, eventID := uuid.New(), uuid.New(), uuid.New()
	authorizer := &workflowAuthorizer{}
	workflow := &workflowTransitioner{status: ordersDomain.StatusPaid}
	publisher := &workflowPublisher{}
	facade, err := NewOrdersAdminFacade(authorizer, workflow, workflowTransaction{}, publisher)
	if err != nil {
		t.Fatalf("NewOrdersAdminFacade() error = %v", err)
	}
	facade.WithStatusWorkflow(workflowPolicy{}, workflow)

	err = facade.Transition(context.Background(), OrderTransitionCommand{
		ActorUserID: actor, EventKey: eventID, OrderID: orderID, ToStatusCode: "processing",
	})
	if err != nil {
		t.Fatalf("Transition() error = %v", err)
	}
	if workflow.transition.ToStatusCode != "processing" || workflow.transition.FromStatusCode != ordersDomain.StatusPaid {
		t.Fatalf("workflow transition = %+v", workflow.transition)
	}
	if len(authorizer.permissions) != 2 || authorizer.permissions[0] != PermissionOrdersWrite || authorizer.permissions[1] != "orders:fulfillment:write" {
		t.Fatalf("permissions = %v", authorizer.permissions)
	}
	if publisher.event == nil || publisher.event.Topic() != "admin.action.v1" {
		t.Fatalf("audit event = %#v", publisher.event)
	}
}

func TestOrdersAdminFacadeListsStatusHistoryWithWorkflowReadPermission(t *testing.T) {
	actor, orderID := uuid.New(), uuid.New()
	authorizer := &workflowAuthorizer{}
	workflow := &workflowTransitioner{}
	facade, err := NewOrdersAdminFacade(authorizer, workflow, workflowTransaction{}, &workflowPublisher{})
	if err != nil {
		t.Fatalf("NewOrdersAdminFacade() error = %v", err)
	}
	facade.WithWorkflowConfiguration(&workflowConfiguration{history: []ordersDomain.OrderStatusHistory{{OrderID: orderID, ToStatusCode: "paid"}}})

	history, err := facade.ListOrderStatusHistory(context.Background(), actor, orderID)
	if err != nil {
		t.Fatalf("ListOrderStatusHistory() error = %v", err)
	}
	if len(history) != 1 || history[0].ToStatusCode != "paid" {
		t.Fatalf("history = %#v", history)
	}
	if len(authorizer.permissions) != 1 || authorizer.permissions[0] != "orders:workflow:read" {
		t.Fatalf("permissions = %v", authorizer.permissions)
	}
}

func TestOrdersAdminFacadeCancellationCarriesActorAndReason(t *testing.T) {
	actor, orderID, eventID := uuid.New(), uuid.New(), uuid.New()
	workflow := &workflowTransitioner{}
	facade, err := NewOrdersAdminFacade(&workflowAuthorizer{}, workflow, workflowTransaction{}, &workflowPublisher{})
	if err != nil {
		t.Fatalf("NewOrdersAdminFacade() error = %v", err)
	}
	if err := facade.Cancel(context.Background(), actor, eventID, orderID, "127.0.0.1", "customer requested cancellation"); err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
	if workflow.cancellation.ActorID != actor || workflow.cancellation.OrderID != orderID || workflow.cancellation.Reason != "customer requested cancellation" || workflow.cancellation.EventID != eventID {
		t.Fatalf("cancellation audit = %#v", workflow.cancellation)
	}
}

type workflowAuthorizer struct{ permissions []string }

func (a *workflowAuthorizer) Require(_ context.Context, _ uuid.UUID, permission string) error {
	a.permissions = append(a.permissions, permission)
	return nil
}

type workflowTransaction struct{}

func (workflowTransaction) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type workflowTransitioner struct {
	status       string
	transition   workflowDomain.OperationalStatusTransition
	cancellation workflowDomain.AdminCancellation
}

func (w *workflowTransitioner) CancelPendingWithActor(_ context.Context, cancellation workflowDomain.AdminCancellation) error {
	w.cancellation = cancellation
	return nil
}
func (w *workflowTransitioner) CurrentStatusForUpdate(context.Context, uuid.UUID) (string, error) {
	return w.status, nil
}
func (w *workflowTransitioner) TransitionOperational(_ context.Context, transition workflowDomain.OperationalStatusTransition) error {
	w.transition = transition
	return nil
}

type workflowPolicy struct{}

func (workflowPolicy) ValidateTransition(_ context.Context, request ordersDomain.TransitionRequest) (string, error) {
	if request.FromStatusCode != ordersDomain.StatusPaid || request.ToStatusCode != "processing" || request.Trigger != ordersDomain.TransitionTriggerAdmin {
		return "", ordersDomain.ErrWorkflowTransitionInvalid
	}
	return "orders:fulfillment:write", nil
}

type workflowConfiguration struct {
	history []ordersDomain.OrderStatusHistory
}

func (c *workflowConfiguration) ListConfiguration(context.Context) ([]ordersDomain.OrderStatusDefinition, []ordersDomain.OrderStatusTransition, error) {
	return nil, nil, nil
}
func (c *workflowConfiguration) ListStatusHistory(context.Context, uuid.UUID) ([]ordersDomain.OrderStatusHistory, error) {
	return c.history, nil
}
func (*workflowConfiguration) SaveStatusDefinition(context.Context, ordersDomain.OrderStatusDefinition) error {
	return nil
}
func (*workflowConfiguration) SaveStatusTransition(context.Context, ordersDomain.OrderStatusTransition) error {
	return nil
}

type workflowPublisher struct{ event events.DomainEvent }

func (p *workflowPublisher) Publish(_ context.Context, event events.DomainEvent) error {
	p.event = event
	return nil
}

var _ adminDomain.Authorizer = (*workflowAuthorizer)(nil)
