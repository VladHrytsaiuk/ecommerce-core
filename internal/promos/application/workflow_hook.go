package application

import (
	"context"

	workflowDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/core/orderworkflow/domain"
	ordersDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/orders/domain"
	promosDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/promos/domain"
	"github.com/google/uuid"
)

// WorkflowHook implements promotion reservation as a participant in the order
// workflow transaction. It never performs payment-provider I/O.
type WorkflowHook struct{ promos promosDomain.Repository }

func NewWorkflowHook(promos promosDomain.Repository) *WorkflowHook {
	return &WorkflowHook{promos: promos}
}
func (h *WorkflowHook) BeforeCreatePending(ctx context.Context, order *ordersDomain.Order) error {
	if order == nil || order.Promotion == nil {
		return nil
	}
	return h.promos.Reserve(ctx, order.ID, *order.Promotion)
}
func (h *WorkflowHook) BeforeOrderTransition(ctx context.Context, orderID uuid.UUID, target string) error {
	switch target {
	case ordersDomain.StatusPaid:
		return h.promos.Commit(ctx, orderID)
	case ordersDomain.StatusCancelled:
		return h.promos.Release(ctx, orderID)
	default:
		return nil
	}
}

var _ workflowDomain.TransactionHook = (*WorkflowHook)(nil)
