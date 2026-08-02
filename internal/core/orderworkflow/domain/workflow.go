// Package domain defines the atomic order and reservation lifecycle.
package domain

import (
	"context"
	"errors"

	"github.com/google/uuid"

	ordersDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/orders/domain"
)

var (
	ErrReservationUnavailable = errors.New("order reservation is unavailable")
	ErrInvalidOrderTransition = errors.New("invalid order workflow transition")
)

type Repository interface {
	CreatePending(context.Context, *ordersDomain.Order, []uuid.UUID) error
	CancelPending(context.Context, uuid.UUID) error
	MarkPaid(context.Context, uuid.UUID) error
}

type Service interface {
	CreatePending(context.Context, ordersDomain.Draft, []uuid.UUID) (*ordersDomain.Order, error)
	CancelPending(context.Context, uuid.UUID) error
	MarkPaid(context.Context, uuid.UUID) error
}
