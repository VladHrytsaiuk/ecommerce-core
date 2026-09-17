package application

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	events "github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
)

// RefundConfirmedHandler consumes only the verified, local Orders event. It
// never accepts a browser/provider payload directly.
type RefundConfirmedHandler struct{ service *ReturnService }

func NewRefundConfirmedHandler(service *ReturnService) (*RefundConfirmedHandler, error) {
	if service == nil {
		return nil, fmt.Errorf("returns service is required")
	}
	return &RefundConfirmedHandler{service: service}, nil
}
func (*RefundConfirmedHandler) Topic() string { return events.TopicOrderRefunded }
func (h *RefundConfirmedHandler) Handle(ctx context.Context, delivery events.Delivery) error {
	var payload struct {
		Version int       `json:"version"`
		OrderID uuid.UUID `json:"order_id"`
	}
	if delivery.EventID == uuid.Nil || delivery.AggregateID == uuid.Nil || delivery.Topic != events.TopicOrderRefunded || json.Unmarshal(delivery.Payload, &payload) != nil || payload.Version != 1 || payload.OrderID == uuid.Nil || payload.OrderID != delivery.AggregateID {
		return fmt.Errorf("invalid order refunded event")
	}
	return h.service.ConfirmRefunded(ctx, payload.OrderID)
}
