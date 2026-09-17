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

// MaxSubjectRunes, MaxCustomerMessageRunes and MaxAgentReplyRunes bound what
// reaches support_messages.body, whose only constraint is that it is not empty.
//
// The customer bound is the one that was missing: the agent's reply was checked
// at 2000 runes in the service, while a ticket body and a customer follow-up
// were bounded only by the HTTP handler's 32 KiB request cap — so any caller
// that was not that handler had no bound at all. The agent's stays stricter
// because a reply is rendered into an outbound email; a customer describing a
// problem is given more room.
const (
	MaxSubjectRunes         = 255
	MaxCustomerMessageRunes = 5000
	MaxAgentReplyRunes      = 2000
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
