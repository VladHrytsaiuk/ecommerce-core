package application

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	adminDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
	promosDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/promos/domain"
)

const PermissionPromosWrite = "promos:write"

type TransactionManager interface {
	WithinTransaction(context.Context, func(context.Context) error) error
}

// PromosAdminFacade is the sole Admin-to-Promos mutation boundary. It keeps
// authorization, business mutation, and audit publication in one local DB
// transaction without making either module depend on the other's adapter.
type PromosAdminFacade struct {
	authorizer adminDomain.Authorizer
	promos     promosDomain.AdminService
	tx         TransactionManager
	publisher  events.TransactionalEventPublisher
	now        func() time.Time
}

type CreatePromoCommand struct {
	ActorUserID uuid.UUID
	EventKey    uuid.UUID
	IPAddress   string
	Code        promosDomain.Code
}

func NewPromosAdminFacade(authorizer adminDomain.Authorizer, promos promosDomain.AdminService, tx TransactionManager, publisher events.TransactionalEventPublisher) (*PromosAdminFacade, error) {
	if authorizer == nil || promos == nil || tx == nil || publisher == nil {
		return nil, fmt.Errorf("promos admin facade is not configured")
	}
	return &PromosAdminFacade{authorizer: authorizer, promos: promos, tx: tx, publisher: publisher, now: func() time.Time { return time.Now().UTC() }}, nil
}

func (f *PromosAdminFacade) Create(ctx context.Context, command CreatePromoCommand) (*promosDomain.Code, error) {
	if command.ActorUserID == uuid.Nil {
		return nil, adminDomain.ErrNotAdmin
	}
	if err := f.authorizer.Require(ctx, command.ActorUserID, PermissionPromosWrite); err != nil {
		return nil, err
	}
	if command.EventKey == uuid.Nil {
		command.EventKey = uuid.New()
	}
	var created *promosDomain.Code
	if err := f.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		promo, err := f.promos.Create(txCtx, command.Code)
		if err != nil {
			return err
		}
		newPayload, err := json.Marshal(struct {
			ID            uuid.UUID  `json:"id"`
			Code          string     `json:"code"`
			DiscountType  string     `json:"discount_type"`
			DiscountValue int64      `json:"discount_value"`
			Currency      string     `json:"currency,omitempty"`
			IsActive      bool       `json:"is_active"`
			ValidUntil    *time.Time `json:"valid_until,omitempty"`
			UsageLimit    *int       `json:"usage_limit,omitempty"`
		}{promo.ID, promo.Code, promo.DiscountType, promo.DiscountValue, promo.Currency, promo.IsActive, promo.ValidUntil, promo.UsageLimit})
		if err != nil {
			return fmt.Errorf("marshal promo audit payload: %w", err)
		}
		event, err := adminDomain.NewAdminActionEvent(command.EventKey, command.ActorUserID, "promos.create", "promo", promo.ID, nil, newPayload, command.IPAddress, []byte(`{"module":"promos"}`), f.now())
		if err != nil {
			return err
		}
		if err := f.publisher.Publish(txCtx, event); err != nil {
			return fmt.Errorf("publish admin audit event: %w", err)
		}
		created = promo
		return nil
	}); err != nil {
		return nil, err
	}
	return created, nil
}
