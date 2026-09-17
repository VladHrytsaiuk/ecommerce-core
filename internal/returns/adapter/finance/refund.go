// Package finance translates a Return settlement to the selected payment
// gateway. It deliberately waits for the verified provider webhook to change
// the local financial Order state.
package finance

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
	paymentsApp "github.com/VladHrytsaiuk/ecommerce-core/internal/payments/application"
	paymentsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/payments/domain"
	transaction "github.com/VladHrytsaiuk/ecommerce-core/internal/platform/postgres/transaction"
	returns "github.com/VladHrytsaiuk/ecommerce-core/internal/returns/domain"
)

type RefundPort struct {
	db       *gorm.DB
	gateways *paymentsApp.Registry
}

func NewRefundPort(db *gorm.DB, gateways *paymentsApp.Registry) (*RefundPort, error) {
	if db == nil || gateways == nil {
		return nil, fmt.Errorf("returns financial refund dependencies are required")
	}
	return &RefundPort{db: db, gateways: gateways}, nil
}

func (p *RefundPort) InitiateRefund(ctx context.Context, returnID, orderID uuid.UUID, amount money.Money) error {
	if p == nil || p.db == nil || p.gateways == nil || returnID == uuid.Nil || orderID == uuid.Nil || amount.Validate() != nil {
		return fmt.Errorf("invalid financial refund request")
	}
	if _, err := transaction.FromContext(ctx); err == nil {
		return fmt.Errorf("financial refund must run outside a SQL transaction")
	}
	var payment struct {
		Provider, ProviderReference, Currency string
		Amount                                int64
	}
	err := p.db.WithContext(ctx).Raw(`SELECT provider, provider_reference, currency, amount FROM payments WHERE order_id = ? AND status = 'paid' ORDER BY created_at DESC LIMIT 1`, orderID).Scan(&payment).Error
	if err != nil {
		return err
	}
	if strings.TrimSpace(payment.Provider) == "" || strings.TrimSpace(payment.ProviderReference) == "" {
		return fmt.Errorf("paid payment reference not found")
	}
	if payment.Amount != amount.Amount() || !strings.EqualFold(payment.Currency, amount.Currency()) {
		return fmt.Errorf("refund amount does not match paid payment")
	}
	gateway, ok := p.gateways.Get(payment.Provider)
	if !ok {
		return fmt.Errorf("%w: %s", paymentsDomain.ErrGatewayNotEnabled, payment.Provider)
	}
	return gateway.Refund(ctx, paymentsDomain.RefundRequest{PaymentReference: payment.ProviderReference, Amount: amount, IdempotencyKey: returnID.String()})
}

var _ returns.FinancialRefundPort = (*RefundPort)(nil)
