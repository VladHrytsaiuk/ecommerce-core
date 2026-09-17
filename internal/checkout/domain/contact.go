package domain

import (
	"context"
	"errors"

	"github.com/google/uuid"

	cartDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/cart/domain"
)

var (
	ErrInvalidContact = errors.New("invalid checkout contact")
	ErrCartOwnership  = errors.New("checkout cart is not owned by caller")
)

type CaptureContactCommand struct {
	CartID         uuid.UUID
	Owner          cartDomain.Owner
	Email          string
	MarketingOptIn bool
	IPAddress      string
}

type ContactSnapshot struct {
	CartID uuid.UUID
	Email  string
}

type ContactRepository interface {
	UpsertContact(context.Context, ContactSnapshot) error
	FindContact(context.Context, uuid.UUID) (*ContactSnapshot, error)
}

type MarketingConsentWriter interface {
	GrantActiveMarketing(context.Context, *uuid.UUID, string, string) error
}

type TransactionManager interface {
	WithinTransaction(context.Context, func(context.Context) error) error
}

type ContactCaptureService interface {
	CaptureContact(context.Context, CaptureContactCommand) error
}
