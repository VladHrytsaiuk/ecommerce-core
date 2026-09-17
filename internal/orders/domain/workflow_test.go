package domain

import (
	"errors"
	"testing"
)

func TestValidateTransitionEnforcesConfiguredRequirements(t *testing.T) {
	from := OrderStatusDefinition{Code: "processing", Enabled: true, Kind: StatusKindFulfillment}
	to := OrderStatusDefinition{Code: "shipped", Enabled: true, Kind: StatusKindFulfillment}
	transition := OrderStatusTransition{
		FromStatusCode:         from.Code,
		ToStatusCode:           to.Code,
		AllowedTriggers:        []TransitionTrigger{TransitionTriggerAdmin, TransitionTriggerDeliveryWebhook},
		RequiresPayment:        true,
		RequiresTrackingNumber: true,
	}

	request := TransitionRequest{
		FromStatusCode: from.Code,
		ToStatusCode:   to.Code,
		Trigger:        TransitionTriggerAdmin,
	}
	if err := ValidateTransition(from, to, transition, request); !errors.Is(err, ErrWorkflowRequirement) {
		t.Fatalf("missing payment/tracking error = %v", err)
	}

	request.PaymentConfirmed = true
	if err := ValidateTransition(from, to, transition, request); !errors.Is(err, ErrWorkflowRequirement) {
		t.Fatalf("missing tracking error = %v", err)
	}

	request.TrackingNumber = "20450000000000"
	if err := ValidateTransition(from, to, transition, request); err != nil {
		t.Fatalf("valid transition error = %v", err)
	}
}

func TestValidateTransitionRejectsUnsafeOrDisabledPaths(t *testing.T) {
	from := OrderStatusDefinition{Code: "paid", Enabled: true, Kind: StatusKindPayment}
	to := OrderStatusDefinition{Code: "received", Enabled: true, IsTerminal: true, Kind: StatusKindTerminal}
	transition := OrderStatusTransition{
		FromStatusCode:  from.Code,
		ToStatusCode:    to.Code,
		AllowedTriggers: []TransitionTrigger{TransitionTriggerAdmin},
	}
	request := TransitionRequest{FromStatusCode: from.Code, ToStatusCode: to.Code, Trigger: TransitionTriggerDeliveryWebhook}
	if err := ValidateTransition(from, to, transition, request); !errors.Is(err, ErrWorkflowTriggerDenied) {
		t.Fatalf("unexpected trigger error = %v", err)
	}

	from.IsTerminal = true
	request.Trigger = TransitionTriggerAdmin
	if err := ValidateTransition(from, to, transition, request); !errors.Is(err, ErrWorkflowTransitionInvalid) {
		t.Fatalf("terminal source error = %v", err)
	}

	from.IsTerminal = false
	to.Enabled = false
	if err := ValidateTransition(from, to, transition, request); !errors.Is(err, ErrWorkflowStatusDisabled) {
		t.Fatalf("disabled status error = %v", err)
	}
}
