package application

import (
	"context"
	"encoding/json"
	"fmt"
	adminDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
	"github.com/google/uuid"
)

const PermissionOrdersWrite = "orders:write"

type PendingOrderCanceller interface {
	CancelPending(context.Context, uuid.UUID) error
}
type OrdersAdminFacade struct {
	authorizer adminDomain.Authorizer
	workflow   PendingOrderCanceller
	tx         TransactionManager
	publisher  events.TransactionalEventPublisher
}

func NewOrdersAdminFacade(a adminDomain.Authorizer, w PendingOrderCanceller, tx TransactionManager, p events.TransactionalEventPublisher) (*OrdersAdminFacade, error) {
	if a == nil || w == nil || tx == nil || p == nil {
		return nil, fmt.Errorf("orders admin facade is not configured")
	}
	return &OrdersAdminFacade{a, w, tx, p}, nil
}
func (f *OrdersAdminFacade) Cancel(ctx context.Context, actor, key, orderID uuid.UUID, ip string) error {
	if err := f.authorizer.Require(ctx, actor, PermissionOrdersWrite); err != nil {
		return err
	}
	if key == uuid.Nil {
		key = uuid.New()
	}
	return f.tx.WithinTransaction(ctx, func(tx context.Context) error {
		if err := f.workflow.CancelPending(tx, orderID); err != nil {
			return err
		}
		old, _ := json.Marshal(map[string]string{"status": "pending_payment"})
		next, _ := json.Marshal(map[string]string{"status": "cancelled"})
		e, err := adminDomain.NewAdminActionEvent(key, actor, "orders.cancel", "order", orderID, old, next, ip, nil, nowUTC())
		if err != nil {
			return err
		}
		return f.publisher.Publish(tx, e)
	})
}
