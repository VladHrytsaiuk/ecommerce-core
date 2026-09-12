package application

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	events "github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
	notifications "github.com/VladHrytsaiuk/ecommerce-core/internal/notifications/domain"
	returns "github.com/VladHrytsaiuk/ecommerce-core/internal/returns/domain"
)

// StatusChangedNotifier tells the buyer what happened to their return.
//
// Returns published returns.status_changed.v1 to no consumer at all, so a
// customer filed a request, an administrator approved it, the warehouse
// received the goods and the refund went out, and the store said nothing at any
// point. Every other module with a customer-facing lifecycle — support,
// abandoned carts, back-in-stock — schedules mail; this one did not.
//
// It runs off the outbox rather than inside the transition, so a mail failure
// can never roll back an approval or a receipt, and a redelivery cannot send
// twice: the scheduler dedupes on the job's type, recipient and payload, and
// the payload names this exact transition.
type StatusChangedNotifier struct {
	orders    returns.OrderSnapshotReader
	tx        returns.TransactionManager
	scheduler notifications.NotificationScheduler
}

func NewStatusChangedNotifier(orders returns.OrderSnapshotReader, tx returns.TransactionManager, scheduler notifications.NotificationScheduler) (*StatusChangedNotifier, error) {
	if orders == nil || tx == nil || scheduler == nil {
		return nil, fmt.Errorf("returns status notifier dependencies are required")
	}
	return &StatusChangedNotifier{orders: orders, tx: tx, scheduler: scheduler}, nil
}

func (*StatusChangedNotifier) Topic() string { return returns.TopicStatusChanged }

// notifiableStatuses are the transitions a buyer is waiting on. "new" is
// omitted because the customer has just been shown the request they filed, and
// "closed" because it is bookkeeping rather than news.
var notifiableStatuses = map[returns.ReturnStatus]string{
	returns.ReturnStatusApproved: "return_approved",
	returns.ReturnStatusRejected: "return_rejected",
	returns.ReturnStatusReceived: "return_received",
	returns.ReturnStatusRefunded: "return_refunded",
}

func (n *StatusChangedNotifier) Handle(ctx context.Context, delivery events.Delivery) error {
	if n == nil || n.orders == nil || n.tx == nil || n.scheduler == nil {
		return fmt.Errorf("returns status notifier is not configured")
	}
	var payload struct {
		Version    int                  `json:"version"`
		ReturnID   uuid.UUID            `json:"return_id"`
		OrderID    uuid.UUID            `json:"order_id"`
		ToStatus   returns.ReturnStatus `json:"to_status"`
		OccurredAt time.Time            `json:"occurred_at"`
	}
	if err := json.Unmarshal(delivery.Payload, &payload); err != nil || payload.Version != 1 || payload.ReturnID == uuid.Nil || payload.OrderID == uuid.Nil {
		return fmt.Errorf("invalid return status changed payload")
	}
	jobType, notifiable := notifiableStatuses[payload.ToStatus]
	if !notifiable {
		return nil
	}
	snapshot, err := n.orders.GetOrderSnapshot(ctx, payload.OrderID)
	if err != nil {
		return fmt.Errorf("read order snapshot for return notification: %w", err)
	}
	if strings.TrimSpace(snapshot.ContactEmail) == "" {
		// A guest order placed before contact capture has nobody to write to.
		// Acknowledging is correct: retrying will never find an address.
		return nil
	}
	// ScheduleEmail requires the caller's transaction, and dedupes on the job
	// type, recipient and payload — so a redelivered outbox event resolves to
	// the same row instead of a second message.
	return n.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		return n.scheduler.ScheduleEmail(txCtx, jobType, snapshot.Locale, snapshot.ContactEmail, statusNotificationPayload{
			ReturnID: payload.ReturnID,
			OrderID:  payload.OrderID,
			Status:   string(payload.ToStatus),
		})
	})
}

// statusNotificationPayload is what the operator's template renders. It carries
// no product, address or reason: the buyer is told which return moved and to
// what, and looks up the detail in their account.
type statusNotificationPayload struct {
	ReturnID uuid.UUID `json:"return_id"`
	OrderID  uuid.UUID `json:"order_id"`
	Status   string    `json:"status"`
}
