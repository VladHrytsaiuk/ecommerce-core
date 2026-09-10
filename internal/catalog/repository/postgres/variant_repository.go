package postgres

import (
	"context"
	"strings"

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
		PriceAmount: variant.Price.Amount(),
		Currency:    variant.Price.Currency(),
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

// FindActiveForCheckout resolves a sellable variant and its display name.
//
// The name is a presentation snapshot, not a catalog invariant. Joining
// strictly on the requested locale meant a product with no translation for it
// yet returned "not found" and failed the entire checkout, even though the
// product was active, priced and in stock. The lookup now accepts the store
// fallback too and prefers the exact match when both exist.
func (r *VariantRepository) FindActiveForCheckout(ctx context.Context, variantID uuid.UUID, locale, fallbackLocale string) (*domain.CheckoutVariant, error) {
	var record struct {
		VariantID   uuid.UUID `gorm:"column:variant_id"`
		ProductID   uuid.UUID `gorm:"column:product_id"`
		SKU         string
		ProductName string `gorm:"column:product_name"`
		PriceAmount int64  `gorm:"column:price_amount"`
		Currency    string
		WeightGrams int `gorm:"column:weight_grams"`
	}
	if strings.TrimSpace(fallbackLocale) == "" {
		fallbackLocale = locale
	}
	// DISTINCT ON keeps one row per variant; ordering by the exact-locale
	// match first makes the fallback apply only when the translation is absent.
	err := r.db.WithContext(ctx).Raw(`
		SELECT DISTINCT ON (variants.id)
		       variants.id AS variant_id,
		       variants.product_id,
		       COALESCE(variants.sku, '') AS sku,
		       translations.name AS product_name,
		       variants.price_amount,
		       variants.currency,
		       variants.weight_grams
		  FROM product_variants AS variants
		  JOIN products ON products.id = variants.product_id
		  JOIN product_translations AS translations ON translations.product_id = products.id
		 WHERE variants.id = ?
		   AND variants.status = 'active'
		   AND products.status = 'active'
		   AND translations.locale IN (?, ?)
		 ORDER BY variants.id, (translations.locale = ?) DESC`,
		variantID, locale, fallbackLocale, locale).Scan(&record).Error
	if err != nil {
		return nil, err
	}
	if record.VariantID == uuid.Nil {
		return nil, domain.ErrProductNotFound
	}
	price, err := money.NewMoney(record.PriceAmount, record.Currency)
	if err != nil {
		return nil, err
	}
	return &domain.CheckoutVariant{VariantID: record.VariantID, ProductID: record.ProductID, SKU: record.SKU, ProductName: record.ProductName, UnitPrice: price, WeightGrams: record.WeightGrams}, nil
}

var _ domain.VariantRepository = (*VariantRepository)(nil)
