package application

import (
	"context"
	"errors"
	"testing"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/orders/domain"
)

func TestWorkflowServiceValidatesConfiguredEdge(t *testing.T) {
	repository := &fakeWorkflowRepository{policy: domain.TransitionPolicy{
		From: domain.OrderStatusDefinition{Code: "processing", Enabled: true},
		To:   domain.OrderStatusDefinition{Code: "shipped", Enabled: true},
		Transition: domain.OrderStatusTransition{
			FromStatusCode:         "processing",
			ToStatusCode:           "shipped",
			AllowedTriggers:        []domain.TransitionTrigger{domain.TransitionTriggerAdmin},
			RequiresPayment:        true,
			RequiresTrackingNumber: true,
			RequiredPermission:     "orders:fulfillment:write",
		},
	}}
	service, err := NewWorkflowService(repository)
	if err != nil {
		t.Fatalf("NewWorkflowService() error = %v", err)
	}

	permission, err := service.ValidateTransition(context.Background(), domain.TransitionRequest{
		FromStatusCode:   "processing",
		ToStatusCode:     "shipped",
		Trigger:          domain.TransitionTriggerAdmin,
		PaymentConfirmed: true,
		TrackingNumber:   "20450000000000",
	})
	if err != nil || permission != "orders:fulfillment:write" {
		t.Fatalf("ValidateTransition() = (%q, %v)", permission, err)
	}
}

func TestWorkflowServicePropagatesPolicyFailures(t *testing.T) {
	service, err := NewWorkflowService(&fakeWorkflowRepository{err: domain.ErrWorkflowNotFound})
	if err != nil {
		t.Fatalf("NewWorkflowService() error = %v", err)
	}
	_, err = service.ValidateTransition(context.Background(), domain.TransitionRequest{FromStatusCode: "paid", ToStatusCode: "received"})
	if !errors.Is(err, domain.ErrWorkflowNotFound) {
		t.Fatalf("ValidateTransition() error = %v", err)
	}
}

func TestWorkflowServiceRejectsAdministratorDefinedPaymentStatus(t *testing.T) {
	service, err := NewWorkflowService(&fakeWorkflowRepository{})
	if err != nil {
		t.Fatalf("NewWorkflowService() error = %v", err)
	}

	err = service.SaveStatusDefinition(context.Background(), domain.OrderStatusDefinition{
		Code: "awaiting_capture", Name: "Awaiting capture", Kind: domain.StatusKindPayment,
	})
	if err == nil {
		t.Fatal("SaveStatusDefinition() accepted administrator-defined payment state")
	}
}

type fakeWorkflowRepository struct {
	policy domain.TransitionPolicy
	err    error
}

func (r *fakeWorkflowRepository) ListWorkflowConfiguration(_ context.Context) ([]domain.OrderStatusDefinition, []domain.OrderStatusTransition, error) {
	return nil, nil, r.err
}

func (r *fakeWorkflowRepository) SaveStatusDefinition(_ context.Context, _ domain.OrderStatusDefinition) error {
	return r.err
}

func (r *fakeWorkflowRepository) SaveStatusTransition(_ context.Context, _ domain.OrderStatusTransition) error {
	return r.err
}

func (r *fakeWorkflowRepository) LoadTransitionPolicy(_ context.Context, _, _ string) (domain.TransitionPolicy, error) {
	return r.policy, r.err
}

func (r *fakeWorkflowRepository) AppendStatusHistory(_ context.Context, _ domain.OrderStatusHistory) (bool, error) {
	return true, nil
}
