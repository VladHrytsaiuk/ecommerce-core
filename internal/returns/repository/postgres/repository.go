// Package postgres persists Returns-owned RMA records. It never joins Orders,
// Identity, Catalog or Inventory tables.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	transaction "github.com/VladHrytsaiuk/ecommerce-core/internal/platform/postgres/transaction"
	returns "github.com/VladHrytsaiuk/ecommerce-core/internal/returns/domain"
)

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

type requestRecord struct {
	ID, OrderID, CustomerID uuid.UUID
	Status                  string
	RefundMode              string
	CreatedAt, UpdatedAt    time.Time
}

func (requestRecord) TableName() string { return "return_requests" }

type itemRecord struct {
	ID, ReturnRequestID, VariantID uuid.UUID
	Quantity                       int
	Condition, Reason              string
	CreatedAt                      time.Time
}

func (itemRecord) TableName() string { return "return_items" }

type historyRecord struct {
	ID, ReturnRequestID uuid.UUID
	Status, ActorType   string
	ActorID             *uuid.UUID
	Reason              string
	CreatedAt           time.Time
}

func (historyRecord) TableName() string { return "return_status_history" }

func (r *Repository) Create(ctx context.Context, request *returns.ReturnRequest) error {
	if request == nil || request.ID == uuid.Nil || request.OrderID == uuid.Nil || request.CustomerID == uuid.Nil || len(request.Items) == 0 || len(request.History) != 1 {
		return fmt.Errorf("invalid return request")
	}
	db := r.database(ctx)
	if err := db.Create(&requestRecord{ID: request.ID, OrderID: request.OrderID, CustomerID: request.CustomerID, Status: string(request.Status), RefundMode: string(request.RefundMode), CreatedAt: request.CreatedAt, UpdatedAt: request.UpdatedAt}).Error; err != nil {
		return err
	}
	items := make([]itemRecord, 0, len(request.Items))
	for _, item := range request.Items {
		items = append(items, itemRecord{ID: item.ID, ReturnRequestID: request.ID, VariantID: item.VariantID, Quantity: item.Quantity, Condition: string(item.Condition), Reason: item.Reason, CreatedAt: item.CreatedAt})
	}
	if err := db.Create(&items).Error; err != nil {
		return err
	}
	return db.Create(historyFromDomain(request.History[0])).Error
}

func (r *Repository) Get(ctx context.Context, id uuid.UUID) (*returns.ReturnRequest, error) {
	return r.get(ctx, id, false)
}

func (r *Repository) GetForUpdate(ctx context.Context, id uuid.UUID) (*returns.ReturnRequest, error) {
	return r.get(ctx, id, true)
}

func (r *Repository) FindReceivedByOrderForUpdate(ctx context.Context, orderID uuid.UUID) (*returns.ReturnRequest, error) {
	if orderID == uuid.Nil {
		return nil, fmt.Errorf("invalid return order ID")
	}
	if _, err := transaction.FromContext(ctx); err != nil {
		return nil, fmt.Errorf("return request lock requires transaction context")
	}
	var record requestRecord
	if err := r.database(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("order_id = ? AND status = ?", orderID, string(returns.ReturnStatusReceived)).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, returns.ErrReturnRequestNotFound
		}
		return nil, err
	}
	return r.get(ctx, record.ID, false)
}

func (r *Repository) get(ctx context.Context, id uuid.UUID, lock bool) (*returns.ReturnRequest, error) {
	if r == nil || r.db == nil || id == uuid.Nil {
		return nil, fmt.Errorf("invalid return request ID")
	}
	db := r.database(ctx)
	if lock {
		if _, err := transaction.FromContext(ctx); err != nil {
			return nil, fmt.Errorf("return request lock requires transaction context")
		}
		db = db.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var record requestRecord
	if err := db.First(&record, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, returns.ErrReturnRequestNotFound
		}
		return nil, err
	}
	var itemRows []itemRecord
	if err := r.database(ctx).Where("return_request_id = ?", id).Order("created_at, id").Find(&itemRows).Error; err != nil {
		return nil, err
	}
	var historyRows []historyRecord
	if err := r.database(ctx).Where("return_request_id = ?", id).Order("created_at, id").Find(&historyRows).Error; err != nil {
		return nil, err
	}
	request := &returns.ReturnRequest{ID: record.ID, OrderID: record.OrderID, CustomerID: record.CustomerID, Status: returns.ReturnStatus(record.Status), RefundMode: returns.RefundMode(record.RefundMode), CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt, Items: make([]returns.ReturnItem, 0, len(itemRows)), History: make([]returns.ReturnStatusHistory, 0, len(historyRows))}
	for _, row := range itemRows {
		request.Items = append(request.Items, returns.ReturnItem{ID: row.ID, ReturnRequestID: row.ReturnRequestID, VariantID: row.VariantID, Quantity: row.Quantity, Condition: returns.ItemCondition(row.Condition), Reason: row.Reason, CreatedAt: row.CreatedAt})
	}
	for _, row := range historyRows {
		request.History = append(request.History, returns.ReturnStatusHistory{ID: row.ID, ReturnRequestID: row.ReturnRequestID, Status: returns.ReturnStatus(row.Status), ActorType: returns.ActorType(row.ActorType), ActorID: row.ActorID, Reason: row.Reason, CreatedAt: row.CreatedAt})
	}
	return request, nil
}

func (r *Repository) Update(ctx context.Context, request *returns.ReturnRequest, history returns.ReturnStatusHistory) error {
	if request == nil || request.ID == uuid.Nil || history.ID == uuid.Nil || history.ReturnRequestID != request.ID || history.Status != request.Status {
		return fmt.Errorf("invalid return request update")
	}
	result := r.database(ctx).Model(&requestRecord{}).Where("id = ?", request.ID).Updates(map[string]any{"status": string(request.Status), "updated_at": request.UpdatedAt})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return returns.ErrReturnRequestNotFound
	}
	return r.database(ctx).Create(historyFromDomain(history)).Error
}

func (r *Repository) database(ctx context.Context) *gorm.DB {
	if tx, err := transaction.FromContext(ctx); err == nil {
		return tx.WithContext(ctx)
	}
	return r.db.WithContext(ctx)
}

func historyFromDomain(value returns.ReturnStatusHistory) historyRecord {
	return historyRecord{ID: value.ID, ReturnRequestID: value.ReturnRequestID, Status: string(value.Status), ActorType: string(value.ActorType), ActorID: value.ActorID, Reason: value.Reason, CreatedAt: value.CreatedAt}
}

var _ returns.Repository = (*Repository)(nil)
