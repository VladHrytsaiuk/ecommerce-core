// Package ordersnapshot is the Returns anti-corruption adapter for immutable
// Order facts. It returns a deliberately narrow DTO and does not expose an
// Orders repository to the Returns application layer.
package ordersnapshot

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
	returns "github.com/VladHrytsaiuk/ecommerce-core/internal/returns/domain"
)

type Provider struct{ db *gorm.DB }

func NewProvider(db *gorm.DB) *Provider { return &Provider{db: db} }

func (p *Provider) GetOrderSnapshot(ctx context.Context, orderID uuid.UUID) (returns.OrderSnapshot, error) {
	if p == nil || p.db == nil || orderID == uuid.Nil {
		return returns.OrderSnapshot{}, fmt.Errorf("invalid order snapshot request")
	}
	var row struct {
		ID          uuid.UUID
		CustomerID  *uuid.UUID
		Status      string
		TotalAmount int64
		Currency    string
		PaidAt      *time.Time
		DeliveredAt *time.Time
	}
	query := `SELECT o.id, o.customer_id, o.status, o.total_amount, o.currency,
       COALESCE(paid_history.occurred_at, paid_payment.updated_at) AS paid_at,
       COALESCE(delivered.occurred_at, CASE WHEN o.status = 'delivered' THEN o.updated_at ELSE NULL END) AS delivered_at
FROM orders AS o
LEFT JOIN LATERAL (
    SELECT occurred_at FROM order_status_history
    WHERE order_id = o.id AND to_status_code = 'paid'
    ORDER BY occurred_at DESC, id DESC LIMIT 1
) AS paid_history ON TRUE
LEFT JOIN LATERAL (
    SELECT updated_at FROM payments
    WHERE order_id = o.id AND status IN ('paid', 'refunded')
    ORDER BY updated_at DESC, id DESC LIMIT 1
) AS paid_payment ON TRUE
LEFT JOIN LATERAL (
    SELECT occurred_at FROM order_status_history
    WHERE order_id = o.id AND to_status_code = 'delivered'
    ORDER BY occurred_at DESC, id DESC LIMIT 1
) AS delivered ON TRUE
WHERE o.id = ?`
	if err := p.db.WithContext(ctx).Raw(query, orderID).Scan(&row).Error; err != nil {
		return returns.OrderSnapshot{}, err
	}
	if row.ID == uuid.Nil {
		return returns.OrderSnapshot{}, returns.ErrReturnRequestNotFound
	}
	total, err := money.NewMoney(row.TotalAmount, strings.ToUpper(strings.TrimSpace(row.Currency)))
	if err != nil {
		return returns.OrderSnapshot{}, fmt.Errorf("map order total: %w", err)
	}
	var itemRows []struct {
		VariantID *uuid.UUID
		Quantity  int
		Total     int64
		Currency  string
	}
	if err := p.db.WithContext(ctx).Raw(`SELECT variant_id, quantity, total_amount AS total, currency FROM order_items WHERE order_id = ? ORDER BY id`, orderID).Scan(&itemRows).Error; err != nil {
		return returns.OrderSnapshot{}, err
	}
	items := make([]returns.OrderItemSnapshot, 0, len(itemRows))
	for _, item := range itemRows {
		value, valueErr := money.NewMoney(item.Total, strings.ToUpper(strings.TrimSpace(item.Currency)))
		if valueErr != nil {
			return returns.OrderSnapshot{}, fmt.Errorf("map order item total: %w", valueErr)
		}
		items = append(items, returns.OrderItemSnapshot{VariantID: item.VariantID, Quantity: item.Quantity, Total: value})
	}
	return returns.OrderSnapshot{OrderID: row.ID, CustomerID: row.CustomerID, Status: row.Status, PaidAt: row.PaidAt, DeliveredAt: row.DeliveredAt, Total: total, Items: items}, nil
}

var _ returns.OrderSnapshotReader = (*Provider)(nil)
