package readers

import (
	"context"
	"errors"
	cart "github.com/VladHrytsaiuk/ecommerce-core/internal/abandoned_cart/domain"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"time"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/postgres/transaction"
)

type CartReader struct{ db *gorm.DB }

func NewCartReader(db *gorm.DB) *CartReader { return &CartReader{db} }
func (r *CartReader) GetCartState(c context.Context, id uuid.UUID) (cart.CartState, error) {
	var x struct {
		Status    string
		UpdatedAt time.Time
		Items     int64
	}
	e := r.db.WithContext(c).Raw("SELECT c.status,c.updated_at,COUNT(ci.id) AS items FROM carts c LEFT JOIN cart_items ci ON ci.cart_id=c.id WHERE c.id=? GROUP BY c.id", id).Scan(&x).Error
	return cart.CartState{IsActive: x.Status == "active", IsPaid: x.Status == "converted", IsEmpty: x.Items == 0, LastUpdatedAt: x.UpdatedAt}, e
}

type ConsentReader struct{ db *gorm.DB }

func NewConsentReader(db *gorm.DB) *ConsentReader { return &ConsentReader{db} }
func (r *ConsentReader) HasConsent(c context.Context, id *uuid.UUID, email string) (bool, error) {
	var n int64
	query := r.database(c).Table("customer_consents").Where("document_type='marketing' AND withdrawn_at IS NULL")
	if id != nil && *id != uuid.Nil {
		query = query.Where("customer_id=?", *id)
	} else {
		query = query.Where("contact_email=?", email)
	}
	e := query.Count(&n).Error
	return n > 0, e
}

func (r *ConsentReader) database(ctx context.Context) *gorm.DB {
	if tx, err := transaction.FromContext(ctx); err == nil {
		return tx.WithContext(ctx)
	}
	return r.db.WithContext(ctx)
}

// ContactProvider resolves PII only at the consumer boundary. Contact details
// never travel in an Outbox payload.
type ContactProvider struct{ db *gorm.DB }

func NewContactProvider(db *gorm.DB) *ContactProvider { return &ContactProvider{db: db} }

func (p *ContactProvider) ContactForCart(ctx context.Context, cartID uuid.UUID) (*cart.Contact, error) {
	var cartRow struct{ CustomerID *uuid.UUID }
	if err := p.db.WithContext(ctx).Table("carts").Select("customer_id").Where("id = ?", cartID).Take(&cartRow).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if cartRow.CustomerID != nil && *cartRow.CustomerID != uuid.Nil {
		var user struct{ Email string }
		if err := p.db.WithContext(ctx).Table("users").Select("email").Where("id = ?", *cartRow.CustomerID).Take(&user).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, nil
			}
			return nil, err
		}
		return &cart.Contact{CustomerID: cartRow.CustomerID, Email: user.Email}, nil
	}
	var contact struct{ Email string }
	if err := p.db.WithContext(ctx).Table("checkout_contacts").Select("email").Where("cart_id = ?", cartID).Take(&contact).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &cart.Contact{Email: contact.Email}, nil
}

var _ cart.CartRecoveryContactProvider = (*ContactProvider)(nil)
