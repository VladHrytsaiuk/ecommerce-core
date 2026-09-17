package application

import (
	"context"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"

	cartDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/cart/domain"
	checkout "github.com/VladHrytsaiuk/ecommerce-core/internal/checkout/domain"
	events "github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
)

// ContactCaptureService persists only the minimum checkout contact snapshot.
// Its transaction has no network I/O; consent and Outbox persistence join the
// transaction through their narrow ports.
type ContactCaptureService struct {
	repository checkout.ContactRepository
	carts      cartDomain.Service
	consent    checkout.MarketingConsentWriter
	tx         checkout.TransactionManager
	publisher  events.TransactionalEventPublisher
	now        func() time.Time
}

func NewContactCaptureService(repository checkout.ContactRepository, carts cartDomain.Service, consent checkout.MarketingConsentWriter, tx checkout.TransactionManager, publisher events.TransactionalEventPublisher) *ContactCaptureService {
	return &ContactCaptureService{repository: repository, carts: carts, consent: consent, tx: tx, publisher: publisher, now: func() time.Time { return time.Now().UTC() }}
}

func (s *ContactCaptureService) CaptureContact(ctx context.Context, command checkout.CaptureContactCommand) error {
	if s == nil || s.repository == nil || s.carts == nil || s.tx == nil || s.publisher == nil || command.CartID == uuid.Nil {
		return checkout.ErrInvalidContact
	}
	email := strings.ToLower(strings.TrimSpace(command.Email))
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email || len(email) > 320 {
		return checkout.ErrInvalidContact
	}
	ownedCart, err := s.carts.GetOrCreate(ctx, command.Owner)
	if err != nil {
		return fmt.Errorf("resolve checkout cart: %w", err)
	}
	if ownedCart.ID != command.CartID {
		return checkout.ErrCartOwnership
	}
	return s.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := s.repository.UpsertContact(txCtx, checkout.ContactSnapshot{CartID: command.CartID, Email: email}); err != nil {
			return err
		}
		if command.MarketingOptIn {
			if s.consent == nil {
				return checkout.ErrInvalidContact
			}
			if err := s.consent.GrantActiveMarketing(txCtx, command.Owner.CustomerID, email, command.IPAddress); err != nil {
				return err
			}
		}
		event, err := events.NewCheckoutEmailCapturedEvent(command.CartID, s.now())
		if err != nil {
			return err
		}
		return s.publisher.Publish(txCtx, event)
	})
}

var _ checkout.ContactCaptureService = (*ContactCaptureService)(nil)
