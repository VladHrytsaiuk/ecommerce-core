// Package inventory adapts the Inventory application service to the Returns
// port and keeps local stock adjustments idempotent per return line.
package inventory

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	inventoryDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/inventory/domain"
	transaction "github.com/VladHrytsaiuk/ecommerce-core/internal/platform/postgres/transaction"
	returns "github.com/VladHrytsaiuk/ecommerce-core/internal/returns/domain"
)

type RestockPort struct {
	db          *gorm.DB
	inventory   inventoryDomain.Service
	warehouseID uuid.UUID
}

func NewRestockPort(db *gorm.DB, inventory inventoryDomain.Service, warehouseID uuid.UUID) (*RestockPort, error) {
	if db == nil || inventory == nil || warehouseID == uuid.Nil {
		return nil, fmt.Errorf("returns inventory restock dependencies are required")
	}
	return &RestockPort{db: db, inventory: inventory, warehouseID: warehouseID}, nil
}

func (p *RestockPort) RestockItems(ctx context.Context, items []returns.ReturnItem) error {
	if p == nil || p.db == nil || p.inventory == nil {
		return fmt.Errorf("returns inventory restock is not configured")
	}
	return transaction.Within(ctx, p.db, func(tx *gorm.DB) error {
		txCtx := transaction.WithContext(ctx, tx)
		for _, item := range items {
			if item.ID == uuid.Nil || item.ReturnRequestID == uuid.Nil || item.VariantID == uuid.Nil || item.Quantity <= 0 || item.Condition != returns.ItemConditionUnopened {
				return fmt.Errorf("invalid restock return item")
			}
			result := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "return_item_id"}}, DoNothing: true}).Table("return_restock_operations").Create(map[string]any{"return_item_id": item.ID, "return_request_id": item.ReturnRequestID})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				continue
			}
			if err := p.inventory.Adjust(txCtx, item.VariantID, p.warehouseID, item.Quantity); err != nil {
				return err
			}
		}
		return nil
	})
}

var _ returns.InventoryRestockPort = (*RestockPort)(nil)
