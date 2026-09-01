package postgres

import (
	"context"
	consent "github.com/VladHrytsaiuk/ecommerce-core/internal/consent/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/postgres/transaction"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db} }

type document consent.LegalDocument

func (document) TableName() string { return "legal_documents" }

type customerConsent consent.CustomerConsent

func (customerConsent) TableName() string { return "customer_consents" }

type privacy consent.PrivacyRequest

func (privacy) TableName() string { return "privacy_requests" }
func (r *Repository) ActiveDocuments(c context.Context) (v []consent.LegalDocument, e error) {
	var x []document
	e = r.db.WithContext(c).Where("is_active = true").Order("type").Find(&x).Error
	for _, q := range x {
		v = append(v, consent.LegalDocument(q))
	}
	return
}
func (r *Repository) Consents(c context.Context, id uuid.UUID) (v []consent.CustomerConsent, e error) {
	var x []customerConsent
	e = r.db.WithContext(c).Where("customer_id=?", id).Order("granted_at DESC").Find(&x).Error
	for _, q := range x {
		v = append(v, consent.CustomerConsent(q))
	}
	return
}
func (r *Repository) IsActiveDocument(c context.Context, t, v string) (bool, error) {
	var n int64
	e := r.db.WithContext(c).Table("legal_documents").Where("type=? AND version=? AND is_active=true", t, v).Count(&n).Error
	return n == 1, e
}
func (r *Repository) Grant(c context.Context, x consent.CustomerConsent) error {
	return r.db.WithContext(c).Create((*customerConsent)(&x)).Error
}
func (r *Repository) Withdraw(c context.Context, id uuid.UUID, t string, at time.Time) error {
	return r.db.WithContext(c).Model((*customerConsent)(nil)).Where("customer_id=? AND document_type=? AND withdrawn_at IS NULL", id, t).Update("withdrawn_at", at).Error
}
func (r *Repository) CreatePrivacyRequest(c context.Context, x consent.PrivacyRequest) error {
	return r.db.WithContext(c).Create((*privacy)(&x)).Error
}
func (r *Repository) CreateDocument(c context.Context, x consent.LegalDocument) error {
	return r.database(c).Create((*document)(&x)).Error
}
func (r *Repository) PublishDocument(c context.Context, id uuid.UUID) (*consent.LegalDocument, error) {
	if _, e := transaction.FromContext(c); e != nil {
		return nil, e
	}
	var row document
	if e := r.database(c).Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "id=?", id).Error; e != nil {
		return nil, e
	}
	if e := r.database(c).Model((*document)(nil)).Where("type=?", row.Type).Update("is_active", false).Error; e != nil {
		return nil, e
	}
	if e := r.database(c).Model(&row).Updates(map[string]any{"is_active": true, "published_at": time.Now().UTC()}).Error; e != nil {
		return nil, e
	}
	row.IsActive = true
	return (*consent.LegalDocument)(&row), nil
}
func (r *Repository) ListPrivacyRequests(c context.Context, l, o int) (v []consent.PrivacyRequest, n int64, e error) {
	q := r.database(c).Model((*privacy)(nil))
	if e = q.Count(&n).Error; e != nil {
		return
	}
	var x []privacy
	e = q.Order("created_at DESC,id DESC").Limit(l).Offset(o).Find(&x).Error
	for _, a := range x {
		v = append(v, consent.PrivacyRequest(a))
	}
	return
}
func (r *Repository) ApprovePrivacyRequest(c context.Context, id uuid.UUID) (*consent.PrivacyRequest, error) {
	if _, e := transaction.FromContext(c); e != nil {
		return nil, e
	}
	var row privacy
	if e := r.database(c).Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "id=?", id).Error; e != nil {
		return nil, e
	}
	if row.Status != "pending" {
		return nil, consent.ErrInvalidPrivacyTransition
	}
	if e := r.database(c).Model(&row).Update("status", "in_progress").Error; e != nil {
		return nil, e
	}
	row.Status = "in_progress"
	return (*consent.PrivacyRequest)(&row), nil
}
func (r *Repository) database(c context.Context) *gorm.DB {
	if tx, e := transaction.FromContext(c); e == nil {
		return tx.WithContext(c)
	}
	return r.db.WithContext(c)
}

var _ consent.Repository = (*Repository)(nil)
