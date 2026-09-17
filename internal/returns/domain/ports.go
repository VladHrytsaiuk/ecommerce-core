package domain

import (
	"context"
	"time"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
	"github.com/google/uuid"
)

// InventoryRestockPort is implemented by the composition root adapter. Its
// implementation must be idempotent for each ReturnItem.ID.
type InventoryRestockPort interface {
	RestockItems(context.Context, []ReturnItem) error
}

// FinancialRefundPort starts a provider-side refund. It must use returnID as
// its provider idempotency key and must not mark an order refunded: only a
// verified provider callback is financial confirmation.
type FinancialRefundPort interface {
	InitiateRefund(ctx context.Context, returnID uuid.UUID, orderID uuid.UUID, amount money.Money) error
}

// OrderSnapshotReader is the Returns anti-corruption port. Bootstrap supplies
// an adapter; Returns never imports Orders models or repositories.
type OrderSnapshotReader interface {
	GetOrderSnapshot(context.Context, uuid.UUID) (OrderSnapshot, error)
}

// Repository owns all RMA tables. Mutations must receive the transaction
// context opened by TransactionManager.
type Repository interface {
	Create(context.Context, *ReturnRequest) error
	Get(context.Context, uuid.UUID) (*ReturnRequest, error)
	GetForUpdate(context.Context, uuid.UUID) (*ReturnRequest, error)
	FindReceivedByOrderForUpdate(context.Context, uuid.UUID) (*ReturnRequest, error)
	Update(context.Context, *ReturnRequest, ReturnStatusHistory) error
}

type TransactionManager interface {
	WithinTransaction(context.Context, func(context.Context) error) error
}

type OrderItemSnapshot struct {
	VariantID *uuid.UUID
	Quantity  int
	Total     money.Money
}

// Clock is a small test seam; application code uses real UTC time by default.
type Clock interface{ Now() time.Time }
