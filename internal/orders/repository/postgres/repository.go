package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/orders/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/postgres/transaction"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

type orderRecord struct {
	ID               uuid.UUID `gorm:"type:uuid;primaryKey"`
	Number           string
	CustomerID       *uuid.UUID
	Status           string
	Currency         string
	SubtotalAmount   int64
	TaxAmount        int64
	ShippingAmount   int64
	TotalAmount      int64
	PaymentProvider  string
	DeliveryProvider string
	CreatedAt        time.Time
}

func (orderRecord) TableName() string {
	return "orders"
}

type itemRecord struct {
	ID              uuid.UUID `gorm:"type:uuid;primaryKey"`
	OrderID         uuid.UUID
	VariantID       *uuid.UUID
	ProductName     string
	SKU             string
	Quantity        int
	UnitPriceAmount int64
	TotalAmount     int64
	DiscountAmount  int64
	Currency        string
	UnitWeightGrams int
}

func (itemRecord) TableName() string {
	return "order_items"
}

func (r *Repository) Create(ctx context.Context, order *domain.Order) error {
	if tx, err := transaction.FromContext(ctx); err == nil {
		return createInTransaction(tx.WithContext(ctx), order)
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return createInTransaction(tx, order)
	})
}

func (r *Repository) ListByCustomer(ctx context.Context, customerID uuid.UUID, page, limit int) (domain.Page, error) {
	db := r.database(ctx).Where("customer_id = ?", customerID)
	var total int64
	if err := db.Model(&orderRecord{}).Count(&total).Error; err != nil {
		return domain.Page{}, err
	}
	var records []orderRecord
	if err := db.Order("created_at DESC, id DESC").Limit(limit).Offset((page - 1) * limit).Find(&records).Error; err != nil {
		return domain.Page{}, err
	}
	orders := make([]domain.Order, 0, len(records))
	for _, record := range records {
		total, err := moneyFromRecord(record.TotalAmount, record.Currency)
		if err != nil {
			return domain.Page{}, fmt.Errorf("map order %s total: %w", record.ID, err)
		}
		orders = append(orders, domain.Order{ID: record.ID, Number: record.Number, CustomerID: record.CustomerID, Status: record.Status, Total: total, CreatedAt: record.CreatedAt})
	}
	return domain.Page{Orders: orders, Total: total}, nil
}

func (r *Repository) database(ctx context.Context) *gorm.DB {
	if tx, err := transaction.FromContext(ctx); err == nil {
		return tx.WithContext(ctx)
	}
	return r.db.WithContext(ctx)
}

func createInTransaction(tx *gorm.DB, order *domain.Order) error {
	record := orderRecord{
		ID:               order.ID,
		Number:           order.Number,
		CustomerID:       order.CustomerID,
		Status:           order.Status,
		Currency:         order.Total.Currency(),
		SubtotalAmount:   order.Subtotal.Amount(),
		TaxAmount:        order.Tax.Amount(),
		ShippingAmount:   order.Shipping.Amount(),
		TotalAmount:      order.Total.Amount(),
		PaymentProvider:  order.PaymentProvider,
		DeliveryProvider: order.DeliveryProvider,
	}
	if err := tx.Create(&record).Error; err != nil {
		return err
	}

	items := make([]itemRecord, 0, len(order.Items))
	for _, item := range order.Items {
		items = append(items, itemRecord{
			ID:              uuid.New(),
			OrderID:         order.ID,
			VariantID:       item.VariantID,
			ProductName:     item.ProductName,
			SKU:             item.SKU,
			Quantity:        item.Quantity,
			UnitPriceAmount: item.UnitPrice.Amount(),
			TotalAmount:     item.Total.Amount(),
			DiscountAmount:  item.Discount.Amount(),
			Currency:        item.Total.Currency(),
			UnitWeightGrams: item.UnitWeightGrams,
		})
	}
	return tx.Create(&items).Error
}

var _ domain.Repository = (*Repository)(nil)

func moneyFromRecord(amount int64, currency string) (money.Money, error) {
	value, err := money.NewMoney(amount, currency)
	if err != nil {
		return money.Money{}, fmt.Errorf("invalid persisted money: %w", err)
	}
	return value, nil
}
