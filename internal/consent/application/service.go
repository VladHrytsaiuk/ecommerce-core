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
}

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
func (s *Service) ApprovePrivacyRequest(c context.Context, id uuid.UUID) (*consent.PrivacyRequest, error) {
	var x *consent.PrivacyRequest
	e := s.tx.WithinTransaction(c, func(tc context.Context) error {
		var e error
		x, e = s.repo.ApprovePrivacyRequest(tc, id)
		if e != nil || x.RequestType != "erasure" {
			return e
		}
		return s.publisher.Publish(tc, consent.ErasureRequestedEvent{EventID: x.ID, CustomerID: x.CustomerID, At: s.now()})
	})
	return x, e
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
func (s *Service) PrivacyRequest(c context.Context, id uuid.UUID, t string) error {
	if t != "export" && t != "erasure" {
		return consent.ErrInvalid
	}
	return s.repo.CreatePrivacyRequest(c, consent.PrivacyRequest{ID: uuid.New(), CustomerID: id, RequestType: t, Status: "pending", CreatedAt: s.now()})
}
