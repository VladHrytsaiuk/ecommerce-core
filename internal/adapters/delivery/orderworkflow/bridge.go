// Package orderworkflow adapts neutral delivery facts to the Order workflow.
package orderworkflow

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	workflowDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/core/orderworkflow/domain"
	deliveryDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/delivery/domain"
	ordersDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/orders/domain"
)

type operationalService interface {
	TransitionOperational(context.Context, workflowDomain.OperationalStatusTransition) error
}

type Bridge struct{ workflow operationalService }

func NewBridge(workflow operationalService) (*Bridge, error) {
	if workflow == nil {
		return nil, fmt.Errorf("order workflow is required")
	}
	return &Bridge{workflow: workflow}, nil
}

func (b *Bridge) TransitionFromDelivery(ctx context.Context, transition deliveryDomain.OrderStatusTransition) error {
	if b == nil || b.workflow == nil || transition.DeliveryID == uuid.Nil || transition.OrderID == uuid.Nil || transition.ToStatusCode == "" {
		return fmt.Errorf("invalid delivery order transition")
	}
	metadata, err := json.Marshal(map[string]string{"trigger": string(ordersDomain.TransitionTriggerDeliveryWebhook), "delivery_id": transition.DeliveryID.String()})
	if err != nil {
		return err
	}
	eventID := uuid.NewSHA1(transition.DeliveryID, []byte("delivery-status:"+transition.ToStatusCode))
	return b.workflow.TransitionOperational(ctx, workflowDomain.OperationalStatusTransition{
		OrderID: transition.OrderID, ToStatusCode: transition.ToStatusCode,
		Trigger: ordersDomain.TransitionTriggerDeliveryWebhook, PaymentConfirmed: true,
		TrackingNumber: transition.TrackingNumber, ActorType: ordersDomain.StatusActorDeliveryProvider,
		Metadata: metadata, EventID: eventID, OccurredAt: transition.OccurredAt,
	})
}

var _ deliveryDomain.OrderTransitioner = (*Bridge)(nil)
