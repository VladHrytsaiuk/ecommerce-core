package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/postgres/transaction"
)

type OptionsRepository struct{ db *gorm.DB }

func NewOptionsRepository(db *gorm.DB) *OptionsRepository { return &OptionsRepository{db: db} }

type optionRecord struct {
	ID        uuid.UUID `gorm:"column:id"`
	ProductID uuid.UUID `gorm:"column:product_id"`
	Name      string    `gorm:"column:name"`
	Position  int       `gorm:"column:position"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (optionRecord) TableName() string { return "product_options" }

type optionValueRecord struct {
	ID        uuid.UUID `gorm:"column:id"`
	OptionID  uuid.UUID `gorm:"column:option_id"`
	ProductID uuid.UUID `gorm:"column:product_id"`
	Value     string    `gorm:"column:value"`
	Position  int       `gorm:"column:position"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (optionValueRecord) TableName() string { return "product_option_values" }

type variantOptionValueRecord struct {
	VariantID     uuid.UUID `gorm:"column:variant_id"`
	OptionValueID uuid.UUID `gorm:"column:option_value_id"`
	OptionID      uuid.UUID `gorm:"column:option_id"`
	ProductID     uuid.UUID `gorm:"column:product_id"`
}

func (variantOptionValueRecord) TableName() string { return "variant_option_values" }

func (r *OptionsRepository) CreateProductOption(ctx context.Context, option *domain.ProductOption) error {
	if r == nil || r.db == nil || option == nil {
		return fmt.Errorf("catalog options repository is not configured")
	}
	return transaction.Within(ctx, r.db, func(tx *gorm.DB) error {
		if option.ID == uuid.Nil {
			option.ID = uuid.New()
		}
		record := optionRecord{ID: option.ID, ProductID: option.ProductID, Name: strings.TrimSpace(option.Name), Position: option.Position}
		if err := tx.Create(&record).Error; err != nil {
			return err
		}
		values := make([]optionValueRecord, 0, len(option.Values))
		for index := range option.Values {
			value := &option.Values[index]
			if value.ID == uuid.Nil {
				value.ID = uuid.New()
			}
			value.OptionID, value.ProductID = option.ID, option.ProductID
			values = append(values, optionValueRecord{ID: value.ID, OptionID: value.OptionID, ProductID: value.ProductID, Value: strings.TrimSpace(value.Value), Position: value.Position})
		}
		if len(values) == 0 {
			return domain.ErrInvalidProduct
		}
		return tx.Create(&values).Error
	})
}

func (r *OptionsRepository) CreateVariantWithOptionValues(ctx context.Context, variant *domain.ProductVariant, optionValueIDs []uuid.UUID) error {
	if r == nil || r.db == nil || variant == nil {
		return fmt.Errorf("catalog options repository is not configured")
	}
	return transaction.Within(ctx, r.db, func(tx *gorm.DB) error {
		var values []optionValueRecord
		if err := tx.Where("product_id = ? AND id IN ?", variant.ProductID, optionValueIDs).Order("option_id, id").Find(&values).Error; err != nil {
			return err
		}
		if len(values) != len(optionValueIDs) {
			return fmt.Errorf("%w: unknown or foreign option value", domain.ErrInvalidProduct)
		}
		variant.OptionValues = make([]domain.ProductOptionValue, 0, len(values))
		seenOptions := make(map[uuid.UUID]struct{}, len(values))
		for _, value := range values {
			if _, exists := seenOptions[value.OptionID]; exists {
				return fmt.Errorf("%w: more than one selected value for an option", domain.ErrInvalidProduct)
			}
			seenOptions[value.OptionID] = struct{}{}
			variant.OptionValues = append(variant.OptionValues, toOptionValue(value))
		}
		if err := variant.ValidateOptionValues(); err != nil {
			return err
		}
		if variant.ID == uuid.Nil {
			variant.ID = uuid.New()
		}
		if err := tx.Create(&variantRecord{ID: variant.ID, ProductID: variant.ProductID, SKU: nullableString(variant.SKU), Barcode: nullableString(variant.Barcode), Status: variant.Status, PriceAmount: variant.Price.Amount(), Currency: variant.Price.Currency(), WeightGrams: variant.WeightGrams}).Error; err != nil {
			return err
		}
		links := make([]variantOptionValueRecord, 0, len(values))
		for _, value := range values {
			links = append(links, variantOptionValueRecord{VariantID: variant.ID, OptionValueID: value.ID, OptionID: value.OptionID, ProductID: variant.ProductID})
		}
		return tx.Create(&links).Error
	})
}

func loadProductOptions(ctx context.Context, db *gorm.DB, product *domain.Product) (map[uuid.UUID]domain.ProductOptionValue, error) {
	var options []optionRecord
	if err := db.WithContext(ctx).Where("product_id = ?", product.ID).Order("position, id").Find(&options).Error; err != nil {
		return nil, err
	}
	product.Options = make([]domain.ProductOption, 0, len(options))
	byID := make(map[uuid.UUID]int, len(options))
	for _, option := range options {
		byID[option.ID] = len(product.Options)
		product.Options = append(product.Options, domain.ProductOption{ID: option.ID, ProductID: option.ProductID, Name: option.Name, Position: option.Position, Values: []domain.ProductOptionValue{}})
	}
	if len(options) == 0 {
		return map[uuid.UUID]domain.ProductOptionValue{}, nil
	}
	ids := make([]uuid.UUID, 0, len(options))
	for _, option := range options {
		ids = append(ids, option.ID)
	}
	var values []optionValueRecord
	if err := db.WithContext(ctx).Where("option_id IN ?", ids).Order("option_id, position, id").Find(&values).Error; err != nil {
		return nil, err
	}
	valuesByID := make(map[uuid.UUID]domain.ProductOptionValue, len(values))
	for _, record := range values {
		value := toOptionValue(record)
		valuesByID[value.ID] = value
		if index, ok := byID[value.OptionID]; ok {
			product.Options[index].Values = append(product.Options[index].Values, value)
		}
	}
	return valuesByID, nil
}

func loadProductVariants(ctx context.Context, db *gorm.DB, product *domain.Product, values map[uuid.UUID]domain.ProductOptionValue) error {
	var records []variantRecord
	if err := db.WithContext(ctx).Where("product_id = ?", product.ID).Order("created_at, id").Find(&records).Error; err != nil {
		return err
	}
	product.Variants = make([]domain.ProductVariant, 0, len(records))
	byID := make(map[uuid.UUID]int, len(records))
	for _, record := range records {
		price, err := money.NewMoney(record.PriceAmount, record.Currency)
		if err != nil {
			return fmt.Errorf("map variant money: %w", err)
		}
		byID[record.ID] = len(product.Variants)
		product.Variants = append(product.Variants, domain.ProductVariant{ID: record.ID, ProductID: record.ProductID, SKU: stringOrEmpty(record.SKU), Barcode: stringOrEmpty(record.Barcode), Status: record.Status, Price: price, WeightGrams: record.WeightGrams, OptionValues: []domain.ProductOptionValue{}})
	}
	if len(records) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, 0, len(records))
	for _, record := range records {
		ids = append(ids, record.ID)
	}
	var links []variantOptionValueRecord
	if err := db.WithContext(ctx).Where("variant_id IN ?", ids).Order("variant_id, option_id, option_value_id").Find(&links).Error; err != nil {
		return err
	}
	for _, link := range links {
		if index, ok := byID[link.VariantID]; ok {
			if value, exists := values[link.OptionValueID]; exists {
				product.Variants[index].OptionValues = append(product.Variants[index].OptionValues, value)
			}
		}
	}
	return nil
}

func toOptionValue(record optionValueRecord) domain.ProductOptionValue {
	return domain.ProductOptionValue{ID: record.ID, OptionID: record.OptionID, ProductID: record.ProductID, Value: record.Value, Position: record.Position}
}
func stringOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

var _ domain.ProductOptionsRepository = (*OptionsRepository)(nil)
