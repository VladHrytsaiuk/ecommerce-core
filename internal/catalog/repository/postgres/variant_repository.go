package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
	transaction "github.com/VladHrytsaiuk/ecommerce-core/internal/platform/postgres/transaction"
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

// FindVariantForUpdate locks the row so an audited update records the state it
// actually replaced rather than one another writer moved in between.
func (r *VariantRepository) FindVariantForUpdate(ctx context.Context, variantID uuid.UUID) (*domain.ProductVariant, error) {
	if variantID == uuid.Nil {
		return nil, domain.ErrInvalidProduct
	}
	var record variantRecord
	if err := r.database(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", variantID).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrProductNotFound
		}
		return nil, err
	}
	return variantFromRecord(record)
}

func (r *VariantRepository) UpdateVariant(ctx context.Context, variantID uuid.UUID, command domain.UpdateVariantCommand) (*domain.ProductVariant, error) {
	if variantID == uuid.Nil {
		return nil, domain.ErrInvalidProduct
	}
	result := r.database(ctx).Model(&variantRecord{}).Where("id = ?", variantID).Updates(map[string]any{
		"sku": nullableString(command.SKU), "barcode": nullableString(command.Barcode),
		"status": command.Status, "price_amount": command.Price.Amount(),
		"currency": command.Price.Currency(), "weight_grams": command.WeightGrams,
		"updated_at": gorm.Expr("CURRENT_TIMESTAMP"),
	})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected != 1 {
		return nil, domain.ErrProductNotFound
	}
	return r.findVariant(ctx, variantID)
}

// ArchiveVariant withdraws the variant from sale. Re-archiving is a no-op so a
// retried admin request does not surface as a missing record.
func (r *VariantRepository) ArchiveVariant(ctx context.Context, variantID uuid.UUID) (*domain.ProductVariant, error) {
	if variantID == uuid.Nil {
		return nil, domain.ErrInvalidProduct
	}
	result := r.database(ctx).Model(&variantRecord{}).Where("id = ? AND status <> ?", variantID, "archived").
		Updates(map[string]any{"status": "archived", "updated_at": gorm.Expr("CURRENT_TIMESTAMP")})
	if result.Error != nil {
		return nil, result.Error
	}
	return r.findVariant(ctx, variantID)
}

func (r *VariantRepository) findVariant(ctx context.Context, variantID uuid.UUID) (*domain.ProductVariant, error) {
	var record variantRecord
	if err := r.database(ctx).Where("id = ?", variantID).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrProductNotFound
		}
		return nil, err
	}
	return variantFromRecord(record)
}

func variantFromRecord(record variantRecord) (*domain.ProductVariant, error) {
	price, err := money.NewMoney(record.PriceAmount, record.Currency)
	if err != nil {
		return nil, fmt.Errorf("map variant %s price: %w", record.ID, err)
	}
	variant := &domain.ProductVariant{
		ID: record.ID, ProductID: record.ProductID, Status: record.Status,
		Price: price, WeightGrams: record.WeightGrams,
	}
	if record.SKU != nil {
		variant.SKU = *record.SKU
	}
	if record.Barcode != nil {
		variant.Barcode = *record.Barcode
	}
	return variant, nil
}

// database joins an outer transaction when the caller supplies one, so an
// audited mutation and its event share a single unit of work.
func (r *VariantRepository) database(ctx context.Context) *gorm.DB {
	if tx, err := transaction.FromContext(ctx); err == nil {
		return tx.WithContext(ctx)
	}
	return r.db.WithContext(ctx)
}

func nullableString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

// FindActiveForCheckoutBatch resolves every sellable variant in one query.
//
// The display name is a presentation snapshot, not a catalog invariant.
// Joining strictly on the requested locale meant a product with no translation
// for it yet returned "not found" and failed the entire checkout, even though
// the product was active, priced and in stock. The lookup accepts the store
// fallback too and prefers the exact match when both exist.
func (r *VariantRepository) FindActiveForCheckoutBatch(ctx context.Context, variantIDs []uuid.UUID, locale, fallbackLocale string) (map[uuid.UUID]domain.CheckoutVariant, error) {
	found := make(map[uuid.UUID]domain.CheckoutVariant, len(variantIDs))
	if len(variantIDs) == 0 {
		return found, nil
	}
	if strings.TrimSpace(fallbackLocale) == "" {
		fallbackLocale = locale
	}
	var records []struct {
		VariantID   uuid.UUID `gorm:"column:variant_id"`
		ProductID   uuid.UUID `gorm:"column:product_id"`
		SKU         string
		ProductName string `gorm:"column:product_name"`
		PriceAmount int64  `gorm:"column:price_amount"`
		Currency    string
		WeightGrams int `gorm:"column:weight_grams"`
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
		 WHERE variants.id IN ?
		   AND variants.status = 'active'
		   AND products.status = 'active'
		   AND translations.locale IN (?, ?)
		 ORDER BY variants.id, (translations.locale = ?) DESC`,
		variantIDs, locale, fallbackLocale, locale).Scan(&records).Error
	if err != nil {
		return nil, err
	}
	for _, record := range records {
		price, err := money.NewMoney(record.PriceAmount, record.Currency)
		if err != nil {
			return nil, fmt.Errorf("map variant %s price: %w", record.VariantID, err)
		}
		found[record.VariantID] = domain.CheckoutVariant{VariantID: record.VariantID, ProductID: record.ProductID, SKU: record.SKU, ProductName: record.ProductName, UnitPrice: price, WeightGrams: record.WeightGrams}
	}
	return found, nil
}

var _ domain.VariantRepository = (*VariantRepository)(nil)
