package application

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	events "github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
	returns "github.com/VladHrytsaiuk/ecommerce-core/internal/returns/domain"
)

// SettlementHandler runs after the received transition commits. It is an
// Outbox consumer, so calls to Inventory and a payment provider occur outside
// the Return-request transaction and are retried by the durable delivery.
type SettlementHandler struct {
	repository returns.Repository
	orders     returns.OrderSnapshotReader
	restock    returns.InventoryRestockPort
	refund     returns.FinancialRefundPort
}

func NewSettlementHandler(repository returns.Repository, orders returns.OrderSnapshotReader, restock returns.InventoryRestockPort, refund returns.FinancialRefundPort) (*SettlementHandler, error) {
	if repository == nil || orders == nil || restock == nil || refund == nil {
		return nil, fmt.Errorf("returns settlement dependencies are required")
	}
	return &SettlementHandler{repository: repository, orders: orders, restock: restock, refund: refund}, nil
}
func (*SettlementHandler) Topic() string { return returns.TopicSettlementRequested }

func (h *SettlementHandler) Handle(ctx context.Context, delivery events.Delivery) error {
	var payload struct {
		Version  int       `json:"version"`
		ReturnID uuid.UUID `json:"return_id"`
		OrderID  uuid.UUID `json:"order_id"`
	}
	if delivery.EventID == uuid.Nil || delivery.AggregateID == uuid.Nil || delivery.Topic != returns.TopicSettlementRequested || json.Unmarshal(delivery.Payload, &payload) != nil || payload.Version != 1 || payload.ReturnID == uuid.Nil || payload.ReturnID != delivery.AggregateID || payload.OrderID == uuid.Nil {
		return fmt.Errorf("invalid return settlement event")
	}
	request, err := h.repository.Get(ctx, payload.ReturnID)
	if err != nil {
		return err
	}
	if request.OrderID != payload.OrderID {
		return fmt.Errorf("return settlement order mismatch")
	}
	if request.Status != returns.ReturnStatusReceived {
		// A manual close/rejection before the worker ran makes the command
		// obsolete; acknowledging it is the safe idempotent result.
		return nil
	}
	if request.RefundMode != returns.RefundModeFull {
		return ErrUnsupportedRefundMode
	}
	items := make([]returns.ReturnItem, 0, len(request.Items))
	for _, item := range request.Items {
		if item.Condition == returns.ItemConditionUnopened {
			items = append(items, item)
		}
	}
	if len(items) > 0 {
		if err := h.restock.RestockItems(ctx, items); err != nil {
			return err
		}
	}
	snapshot, err := h.orders.GetOrderSnapshot(ctx, request.OrderID)
	if err != nil {
		return err
	}
	if snapshot.Total.Validate() != nil {
		return fmt.Errorf("invalid order amount for return settlement")
	}
	return h.refund.InitiateRefund(ctx, request.ID, request.OrderID, snapshot.Total)
}
