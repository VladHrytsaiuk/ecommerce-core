package domain

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

type Ticket struct {
	ID         uuid.UUID
	CustomerID *uuid.UUID
	Email      string
	Subject    string
	Status     string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type Message struct {
	ID         uuid.UUID
	TicketID   uuid.UUID
	SenderType string
	SenderID   *uuid.UUID
	Body       string
	CreatedAt  time.Time
}

var (
	ErrSpam             = errors.New("support spam protection rejected request")
	ErrInvalidTicket    = errors.New("invalid support ticket")
	ErrTicketNotFound   = errors.New("support ticket not found")
	ErrMessageForbidden = errors.New("support ticket message forbidden")
)

const (
	StatusNew             = "new"
	StatusOpen            = "open"
	StatusPendingCustomer = "pending_customer"
	StatusResolved        = "resolved"
	StatusClosed          = "closed"
)

type SpamProtector interface {
	Check(context.Context, string, string, string) error
}
type CustomerEmailReader interface {
	EmailForCustomer(context.Context, uuid.UUID) (string, error)
}
type Repository interface {
	Create(context.Context, Ticket, Message) (*Ticket, error)
	AddCustomerMessage(context.Context, uuid.UUID, uuid.UUID, string) error
	List(context.Context, string, int, int) ([]Ticket, int64, error)
	Get(context.Context, uuid.UUID) (*Ticket, []Message, error)
	AddAgentMessageAndSetPending(context.Context, uuid.UUID, uuid.UUID, string) (*Ticket, error)
	TransitionStatus(context.Context, uuid.UUID, string) (*Ticket, error)
}

type TransactionManager interface {
	WithinTransaction(context.Context, func(context.Context) error) error
}

// CanTransition is deliberately total and data-independent: support ticket
// states are a small operational workflow, not a user-configurable order flow.
func CanTransition(from, to string) bool {
	if from == to {
		return true
	}
	switch from {
	case StatusNew:
		return to == StatusOpen || to == StatusClosed
	case StatusOpen:
		return to == StatusPendingCustomer || to == StatusResolved || to == StatusClosed
	case StatusPendingCustomer:
		return to == StatusOpen || to == StatusResolved || to == StatusClosed
	case StatusResolved:
		return to == StatusClosed || to == StatusOpen
	default:
		return false
	}
}
