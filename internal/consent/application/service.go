package application

import (
	"context"
	consent "github.com/VladHrytsaiuk/ecommerce-core/internal/consent/domain"
	events "github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
	"github.com/google/uuid"
	"strings"
	"time"
)

type Service struct {
	repo      consent.Repository
	orders    consent.OrderActivityReader
	now       func() time.Time
	tx        consent.TransactionManager
	publisher events.TransactionalEventPublisher
	eraser    consent.ErasureExecutor
}

// WithErasure supplies the deployment's implementation of the right to
// erasure. Without it this service refuses erasure requests instead of
// accepting them into a queue nothing drains.
func (s *Service) WithErasure(executor consent.ErasureExecutor) *Service {
	if s != nil && executor != nil {
		s.eraser = executor
	}
	return s
}

// SupportsErasure reports whether this deployment can carry an erasure request
// out. Delivery uses it to describe the capability rather than discover it
// from a failed request.
func (s *Service) SupportsErasure() bool { return s != nil && s.eraser != nil }

func (s *Service) WithAdminWorkflow(tx consent.TransactionManager, p events.TransactionalEventPublisher) *Service {
	s.tx, s.publisher = tx, p
	return s
}
func (s *Service) CreateDocument(c context.Context, t, v, u string) (*consent.LegalDocument, error) {
	x := &consent.LegalDocument{ID: uuid.New(), Type: strings.TrimSpace(t), Version: strings.TrimSpace(v), ContentURL: strings.TrimSpace(u), PublishedAt: s.now(), IsActive: false}
	if x.Type == "" || x.Version == "" || x.ContentURL == "" {
		return nil, consent.ErrInvalid
	}
	return x, s.repo.CreateDocument(c, *x)
}
func (s *Service) PublishDocument(c context.Context, id uuid.UUID) (*consent.LegalDocument, error) {
	var x *consent.LegalDocument
	e := s.tx.WithinTransaction(c, func(tc context.Context) error { var e error; x, e = s.repo.PublishDocument(tc, id); return e })
	return x, e
}
func (s *Service) ListPrivacyRequests(c context.Context, p, l int) ([]consent.PrivacyRequest, int64, error) {
	return s.repo.ListPrivacyRequests(c, l, (p-1)*l)
}

// ApprovePrivacyRequest marks the request approved and, for an erasure,
// performs it — in one transaction with the audit event.
//
// It used to only publish the event. Nothing consumed that topic, so an
// administrator approved a deletion, the customer was told it was approved,
// and the data stayed exactly where it was. Approving an erasure this
// deployment cannot perform is now refused, which leaves the request pending
// and visible instead of closing it with a lie.
func (s *Service) ApprovePrivacyRequest(c context.Context, id uuid.UUID) (*consent.PrivacyRequest, error) {
	var x *consent.PrivacyRequest
	e := s.tx.WithinTransaction(c, func(tc context.Context) error {
		var e error
		x, e = s.repo.ApprovePrivacyRequest(tc, id)
		if e != nil || x.RequestType != "erasure" {
			return e
		}
		if s.eraser == nil {
			// Rolls the approval back: the request stays pending.
			return consent.ErrErasureUnsupported
		}
		if e := s.eraser.Erase(tc, x.CustomerID); e != nil {
			return e
		}
		return s.publisher.Publish(tc, consent.ErasureRequestedEvent{EventID: x.ID, CustomerID: x.CustomerID, At: s.now()})
	})
	if e != nil {
		return nil, e
	}
	return x, nil
}

func NewService(r consent.Repository, o consent.OrderActivityReader) *Service {
	return &Service{repo: r, orders: o, now: func() time.Time { return time.Now().UTC() }}
}
func (s *Service) ActiveDocuments(c context.Context) ([]consent.LegalDocument, error) {
	return s.repo.ActiveDocuments(c)
}
func (s *Service) Consents(c context.Context, id uuid.UUID) ([]consent.CustomerConsent, error) {
	return s.repo.Consents(c, id)
}
func (s *Service) Grant(c context.Context, id uuid.UUID, t, v, ip string) error {
	ok, e := s.repo.IsActiveDocument(c, strings.TrimSpace(t), strings.TrimSpace(v))
	if e != nil {
		return e
	}
	if !ok {
		return consent.ErrDocumentInactive
	}
	return s.repo.Grant(c, consent.CustomerConsent{ID: uuid.New(), CustomerID: id, DocumentType: strings.TrimSpace(t), DocumentVersion: strings.TrimSpace(v), GrantedAt: s.now(), IPAddress: ip})
}

// GrantActiveMarketing records consent for the currently published marketing
// document. It is intentionally usable inside a caller-owned transaction.
func (s *Service) GrantActiveMarketing(c context.Context, customerID *uuid.UUID, email, ip string) error {
	if (customerID == nil || *customerID == uuid.Nil) && strings.TrimSpace(email) == "" {
		return consent.ErrInvalid
	}
	documents, err := s.repo.ActiveDocuments(c)
	if err != nil {
		return err
	}
	for _, document := range documents {
		if document.Type == "marketing" {
			var id uuid.UUID
			if customerID != nil {
				id = *customerID
			}
			var contactEmail *string
			if id == uuid.Nil {
				normalized := strings.ToLower(strings.TrimSpace(email))
				contactEmail = &normalized
			}
			return s.repo.Grant(c, consent.CustomerConsent{ID: uuid.New(), CustomerID: id, ContactEmail: contactEmail, DocumentType: "marketing", DocumentVersion: document.Version, GrantedAt: s.now(), IPAddress: ip})
		}
	}
	return consent.ErrDocumentInactive
}
func (s *Service) Withdraw(c context.Context, id uuid.UUID, t string) error {
	if t == "terms" {
		active, e := s.orders.HasActiveOrders(c, id)
		if e != nil {
			return e
		}
		if active {
			return consent.ErrTermsWithdrawalBlocked
		}
	}
	return s.repo.Withdraw(c, id, strings.TrimSpace(t), s.now())
}

// WithdrawMarketingByEmail supports a signed unsubscribe endpoint for a guest
// contact without granting that caller access to customer-scoped consent data.
// The HTTP capability/token transport is intentionally left to the delivery
// module; this use case is the domain-safe persistence boundary.
func (s *Service) WithdrawMarketingByEmail(c context.Context, email string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return consent.ErrInvalid
	}
	return s.repo.WithdrawMarketingByEmail(c, email, s.now())
}
func (s *Service) PrivacyRequest(c context.Context, id uuid.UUID, t string) error {
	if t != "export" && t != "erasure" {
		return consent.ErrInvalid
	}
	// Refused at intake, not only at approval: a customer who asks to be
	// deleted should be told now that this store cannot do it, rather than
	// wait for an approval that can never honestly come.
	if t == "erasure" && s.eraser == nil {
		return consent.ErrErasureUnsupported
	}
	return s.repo.CreatePrivacyRequest(c, consent.PrivacyRequest{ID: uuid.New(), CustomerID: id, RequestType: t, Status: "pending", CreatedAt: s.now()})
}
