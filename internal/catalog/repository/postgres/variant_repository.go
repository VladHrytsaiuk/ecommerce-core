package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
)

type VariantRepository struct {
	db *gorm.DB
}

func NewVariantRepository(db *gorm.DB) *VariantRepository {
	return &VariantRepository{db: db}
}

type variantRecord struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey"`
	ProductID   uuid.UUID
	SKU         *string
	Barcode     *string
	Status      string
	PriceAmount int64
	Currency    string
	WeightGrams int
}

func (variantRecord) TableName() string {
	return "product_variants"
}

func (r *VariantRepository) CreateVariant(ctx context.Context, variant *domain.ProductVariant) error {
	record := variantRecord{
		ID:          variant.ID,
		ProductID:   variant.ProductID,
		SKU:         nullableString(variant.SKU),
		Barcode:     nullableString(variant.Barcode),
		Status:      variant.Status,
		PriceAmount: variant.Price.Amount,
		Currency:    variant.Price.Currency,
		WeightGrams: variant.WeightGrams,
	}
	return r.db.WithContext(ctx).Create(&record).Error
}

func nullableString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func (r *VariantRepository) FindActiveForCheckout(ctx context.Context, variantID uuid.UUID, locale string) (*domain.CheckoutVariant, error) {
	var record struct {
		VariantID   uuid.UUID `gorm:"column:variant_id"`
		ProductID   uuid.UUID `gorm:"column:product_id"`
		SKU         string
		ProductName string `gorm:"column:product_name"`
		PriceAmount int64  `gorm:"column:price_amount"`
		Currency    string
		WeightGrams int `gorm:"column:weight_grams"`
	}
	err := r.db.WithContext(ctx).
		Table("product_variants AS variants").
		Select(`variants.id AS variant_id, variants.product_id, COALESCE(variants.sku, '') AS sku,
			product_translations.name AS product_name, variants.price_amount, variants.currency, variants.weight_grams`).
		Joins("JOIN products ON products.id = variants.product_id").
		Joins("JOIN product_translations ON product_translations.product_id = products.id").
		Where("variants.id = ? AND variants.status = ? AND products.status = ? AND product_translations.locale = ?", variantID, "active", "active", locale).
		First(&record).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrProductNotFound
		}
		return nil, err
	}
	price, err := money.New(record.PriceAmount, record.Currency)
	if err != nil {
		return nil, err
	}
	return &domain.CheckoutVariant{VariantID: record.VariantID, ProductID: record.ProductID, SKU: record.SKU, ProductName: record.ProductName, UnitPrice: price, WeightGrams: record.WeightGrams}, nil
}

var _ domain.VariantRepository = (*VariantRepository)(nil)
