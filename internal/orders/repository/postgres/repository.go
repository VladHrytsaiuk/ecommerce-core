package postgres

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/orders/domain"
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
	Currency        string
	UnitWeightGrams int
}

func (itemRecord) TableName() string {
	return "order_items"
}

func (r *Repository) Create(ctx context.Context, order *domain.Order) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return createInTransaction(tx, order)
	})
}

func createInTransaction(tx *gorm.DB, order *domain.Order) error {
	record := orderRecord{
		ID:               order.ID,
		Number:           order.Number,
		CustomerID:       order.CustomerID,
		Status:           order.Status,
		Currency:         order.Total.Currency,
		SubtotalAmount:   order.Subtotal.Amount,
		TaxAmount:        order.Tax.Amount,
		ShippingAmount:   order.Shipping.Amount,
		TotalAmount:      order.Total.Amount,
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
			UnitPriceAmount: item.UnitPrice.Amount,
			TotalAmount:     item.Total.Amount,
			Currency:        item.Total.Currency,
			UnitWeightGrams: item.UnitWeightGrams,
		})
	}
	return tx.Create(&items).Error
}

var _ domain.Repository = (*Repository)(nil)
