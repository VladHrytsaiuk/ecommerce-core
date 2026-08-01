package domain

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	categoryDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/category/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/pagination"
	"gorm.io/gorm"
)

var (
	ErrProductNotFound    = errors.New("product not found")
	ErrSlugNotFound       = errors.New("product with this slug not found")
	ErrReviewNotFound     = errors.New("review not found")
	ErrBrandNotFound      = errors.New("brand not found")
	ErrImageNotFound      = errors.New("image not found")
	ErrBadgeNotFound      = errors.New("badge not found")
	ErrBadgeInUse         = errors.New("cannot delete badge: it is linked to existing products or variations")
	ErrInvalidRating      = errors.New("rating must be between 1 and 5")
	ErrCommentTooLong     = errors.New("comment is too long (max 2000 characters)")
	ErrNestedBundle       = errors.New("a bundle cannot contain another bundle")
	ErrBundleNoComponents = errors.New("a bundle must have at least one component")
)

// LocalizedMap допоміжний тип для роботи з JSONB перекладами у БД
type LocalizedMap map[string]string

// Value реалізує інтерфейс driver.Valuer для запису в БД (перетворює map у JSON)
func (m LocalizedMap) Value() (driver.Value, error) {
	if m == nil {
		return nil, nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

// Scan реалізує інтерфейс sql.Scanner для читання з БД (декодує JSON у map)
func (m *LocalizedMap) Scan(value interface{}) error {
	if value == nil {
		*m = nil
		return nil
	}

	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return fmt.Errorf("failed to scan LocalizedMap: expected []byte or string, got %T", value)
	}

	result := make(map[string]string)
	if err := json.Unmarshal(bytes, &result); err != nil {
		return err
	}
	*m = LocalizedMap(result)
	return nil
}

// Unit одиниця виміру (шт, кг, мл тощо)
type Unit struct {
	ID        int            `gorm:"primaryKey" json:"id"`
	Name      LocalizedMap   `gorm:"type:jsonb;not null" json:"name"`
	ShortName LocalizedMap   `gorm:"type:jsonb;not null" json:"short_name"`
	CreatedAt time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (Unit) TableName() string {
	return "unit"
}

// Brand представляє бренд товару
type Brand struct {
	ID        uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	Name      string         `gorm:"type:varchar(100);not null" json:"name"`
	Slug      string         `gorm:"type:varchar(255);not null" json:"slug"`
	CreatedAt time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (Brand) TableName() string {
	return "brand"
}

// Badge бейдж товару ("Новинка", "Акція" тощо)
type Badge struct {
	ID        int          `gorm:"primaryKey" json:"id"`
	Name      LocalizedMap `gorm:"type:jsonb;not null" json:"name"`
	ColorHex  string       `gorm:"type:varchar(10);not null" json:"color_hex"`
	SortOrder int          `gorm:"not null;default:0" json:"sort_order"`
	CreatedAt time.Time    `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
}

func (Badge) TableName() string {
	return "badge"
}

// ProductBadge зв'язок бейджа з товаром або варіацією
type ProductBadge struct {
	ProductID   *uuid.UUID `gorm:"type:uuid" json:"product_id"`
	VariationID *uuid.UUID `gorm:"type:uuid" json:"variation_id"`
	BadgeID     int        `gorm:"not null" json:"badge_id"`
	Badge       Badge      `gorm:"foreignKey:BadgeID" json:"badge"`
}

func (ProductBadge) TableName() string {
	return "product_badge"
}

// Product основна сутність товару
type Product struct {
	ID            uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	BrandID       uuid.UUID      `gorm:"type:uuid" json:"brand_id"`
	CategoryID    uuid.UUID      `gorm:"type:uuid" json:"category_id"`
	IsActive      bool           `gorm:"not null;default:true" json:"is_active"`
	IsBundle      bool           `gorm:"not null;default:false" json:"is_bundle"`
	PriceStrategy string         `gorm:"type:varchar(20);not null;default:'manual'" json:"price_strategy"`
	AverageRating float64        `gorm:"not null;default:0" json:"average_rating"`
	ReviewsCount  int            `gorm:"not null;default:0" json:"reviews_count"`
	IsRecommended bool           `gorm:"not null;default:false" json:"is_recommended"`
	CreatedAt     time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt     time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`

	// Relationships
	Brand           Brand                   `gorm:"foreignKey:BrandID" json:"brand,omitempty"`
	Category        categoryDomain.Category `gorm:"foreignKey:CategoryID" json:"category,omitempty"`
	Translations    []ProductTranslation    `gorm:"foreignKey:ProductID;constraint:OnDelete:CASCADE;" json:"translations,omitempty"`
	Variations      []ProductVariation      `gorm:"foreignKey:ProductID;constraint:OnDelete:CASCADE;" json:"variations,omitempty"`
	Images          []ProductImage          `gorm:"foreignKey:ProductID;constraint:OnDelete:CASCADE;" json:"images,omitempty"`
	Reviews         []ProductReview         `gorm:"foreignKey:ProductID" json:"reviews,omitempty"`
	AttributeValues []AttributeValue        `gorm:"foreignKey:ProductID;constraint:OnDelete:CASCADE;" json:"attribute_values,omitempty"`
	Badges          []ProductBadge          `gorm:"foreignKey:ProductID" json:"badges,omitempty"`
	ComputedBadges  []Badge                 `gorm:"-" json:"computed_badges,omitempty"`
	BundleItems     []ProductBundleItem     `gorm:"foreignKey:BundleID;constraint:OnDelete:CASCADE;" json:"bundle_items,omitempty"`
}

func (Product) TableName() string {
	return "product"
}

// ProductBundleItem зв'язок набору з варіацією-компонентом
type ProductBundleItem struct {
	BundleID    uuid.UUID        `gorm:"type:uuid;primaryKey" json:"bundle_id"`
	VariationID uuid.UUID        `gorm:"type:uuid;primaryKey" json:"variation_id"`
	Quantity    int              `gorm:"not null;default:1" json:"quantity"`
	Variation   ProductVariation `gorm:"foreignKey:VariationID" json:"variation,omitempty"`
}

func (ProductBundleItem) TableName() string {
	return "product_bundle_item"
}

// ProductTranslation локалізовані дані товару
type ProductTranslation struct {
	ProductID         uuid.UUID `gorm:"type:uuid;primaryKey" json:"product_id"`
	LanguageCode      string    `gorm:"type:varchar(2);primaryKey" json:"language_code"`
	Name              string    `gorm:"type:varchar(255);not null" json:"name"`
	Slug              string    `gorm:"type:varchar(255);not null" json:"slug"`
	Description       string    `gorm:"type:text" json:"description"`
	UsageInstructions string    `gorm:"type:text" json:"usage_instructions"`
	MetaTitle         string    `gorm:"type:varchar(255)" json:"meta_title"`
	MetaDescription   string    `gorm:"type:text" json:"meta_description"`
	MetaKeywords      string    `gorm:"type:varchar(255)" json:"meta_keywords"`
}

func (ProductTranslation) TableName() string {
	return "product_translation"
}

// ProductVariation ціна та залишки конкретної варіації товару
type ProductVariation struct {
	ID            uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	ProductID     uuid.UUID      `gorm:"type:uuid;not null" json:"product_id"`
	Slug          string         `gorm:"type:varchar(255);not null" json:"slug"`
	Name          LocalizedMap   `gorm:"type:jsonb" json:"name"` // опційна власна назва варіації (локалізована); порожня → назва товару
	SKU           string         `gorm:"type:varchar(100)" json:"sku"`
	Barcode       string         `gorm:"type:varchar(50)" json:"barcode"`
	Price         int            `gorm:"not null;default:0" json:"price"` // у мінімальних одиницях (напр. копійки)
	OldPrice      *int           `json:"old_price"`
	QuantityValue float64        `gorm:"type:numeric(10,2)" json:"quantity_value"`
	Weight        float64        `gorm:"type:numeric(10,3);not null;default:0.5" json:"weight"`
	UnitID        *int           `json:"unit_id"`
	IsActive      bool           `gorm:"not null;default:true" json:"is_active"`
	CreatedAt     time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt     time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`

	// Relationships
	Product         Product          `gorm:"foreignKey:ProductID" json:"product,omitempty"`
	Unit            Unit             `gorm:"foreignKey:UnitID" json:"unit,omitempty"`
	AttributeValues []AttributeValue `gorm:"foreignKey:VariationID;constraint:OnDelete:CASCADE;" json:"attribute_values,omitempty"`
	Badges          []ProductBadge   `gorm:"foreignKey:VariationID" json:"badges,omitempty"`
	ComputedBadges  []Badge          `gorm:"-" json:"computed_badges,omitempty"`
}

// ProductVariationUpdate використовується для часткового оновлення варіації
type ProductVariationUpdate struct {
	ID              *uuid.UUID
	Name            *LocalizedMap // nil = не чіпати; інакше — повна заміна локалізованої назви варіації
	SKU             *string
	Barcode         *string
	Price           *int
	OldPrice        *int
	QuantityValue   *float64
	Weight          *float64
	UnitID          *int
	IsActive        *bool
	AttributeValues *[]AttributeValue
	BadgeIDs        *[]int
}

func (ProductVariation) TableName() string {
	return "product_variation"
}

// ProductImage фото товару
type ProductImage struct {
	ID          uuid.UUID    `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	ProductID   uuid.UUID    `gorm:"type:uuid;not null" json:"product_id"`
	VariationID *uuid.UUID   `gorm:"type:uuid" json:"variation_id"`
	ImageURL    string       `gorm:"type:varchar(255);not null" json:"image_url"`
	AltText     LocalizedMap `gorm:"type:jsonb" json:"alt_text"`
	IsPrimary   bool         `gorm:"not null;default:false" json:"is_primary"`
	IsHover     bool         `gorm:"not null;default:false" json:"is_hover"`
	SortOrder   int          `gorm:"not null;default:0" json:"sort_order"`
	CreatedAt   time.Time    `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt   time.Time    `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
}

func (ProductImage) TableName() string {
	return "product_image"
}

// ProductReview відгуки користувачів
type ProductReview struct {
	ID            uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	ProductID     uuid.UUID      `gorm:"type:uuid;not null" json:"product_id"`
	UserID        uuid.UUID      `gorm:"type:uuid;not null" json:"user_id"`
	ParentID      *uuid.UUID     `gorm:"type:uuid" json:"parent_id"`
	Rating        int            `gorm:"type:smallint" json:"rating"`
	Comment       string         `gorm:"type:text" json:"comment"`
	Status        string         `gorm:"type:varchar(20);not null;default:'pending'" json:"status"`
	RejectReason  *string        `gorm:"type:text" json:"reject_reason,omitempty"`
	CreatedAt     time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt     time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
	UserFirstName string         `gorm:"->" json:"user_first_name,omitempty"`
	UserLastName  string         `gorm:"->" json:"user_last_name,omitempty"`
	ProductName   string         `gorm:"->" json:"product_name,omitempty"`
	ProductSlug   string         `gorm:"->" json:"product_slug,omitempty"`
}

func (ProductReview) TableName() string {
	return "product_review"
}

// Attribute - характеристика товару
type Attribute struct {
	ID                int                    `gorm:"primaryKey" json:"id"`
	Code              string                 `gorm:"type:varchar(50);unique" json:"code"`
	SortOrder         int                    `gorm:"not null;default:0" json:"sort_order"`
	IsFilterable      bool                   `gorm:"not null;default:false" json:"is_filterable"`
	IsVariantSpecific bool                   `gorm:"not null;default:false" json:"is_variant_specific"`
	UnitID            *int                   `json:"unit_id"`
	CreatedAt         time.Time              `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt         time.Time              `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
	DeletedAt         gorm.DeletedAt         `gorm:"index" json:"-"`
	Unit              *Unit                  `gorm:"foreignKey:UnitID" json:"unit,omitempty"`
	Translations      []AttributeTranslation `gorm:"foreignKey:AttributeID" json:"translations,omitempty"`
}

func (Attribute) TableName() string {
	return "attribute"
}

// AttributeTranslation - переклад назви атрибута
type AttributeTranslation struct {
	AttributeID  int    `gorm:"primaryKey" json:"attribute_id"`
	LanguageCode string `gorm:"type:varchar(2);primaryKey" json:"language_code"`
	Name         string `gorm:"type:varchar(100);not null" json:"name"`
}

func (AttributeTranslation) TableName() string {
	return "attribute_translation"
}

// AttributeValue - значення атрибута для конкретного товару або варіації
type AttributeValue struct {
	ID           uuid.UUID    `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	AttributeID  int          `gorm:"not null" json:"attribute_id"`
	ValueCode    string       `gorm:"type:varchar(100)" json:"value_code"`
	ValueString  LocalizedMap `gorm:"type:jsonb" json:"value_string"`
	ValueNumeric *float64     `gorm:"type:numeric(10,2)" json:"value_numeric"`
	UnitID       *int         `json:"unit_id"`
	ProductID    *uuid.UUID   `gorm:"type:uuid" json:"product_id"`
	VariationID  *uuid.UUID   `gorm:"type:uuid" json:"variation_id"`

	Attribute Attribute `gorm:"foreignKey:AttributeID" json:"attribute,omitempty"`
	Unit      *Unit     `gorm:"foreignKey:UnitID" json:"unit,omitempty"`
}

func (AttributeValue) TableName() string {
	return "attribute_value"
}

// ProductFilter для передачі параметрів фільтрації
type ProductFilter struct {
	CategoryID     []uuid.UUID
	BrandID        []uuid.UUID
	BrandSlugs     []string
	MinPrice       *int
	MaxPrice       *int
	QuantityValues []string // Дискретні значення (чексбокси)
	AttrValues     map[string][]string
	SearchQuery    string // Текстовий пошук (за назвою, SKU тощо)
}

// FilterDiscovery результат пошуку доступних фільтрів
type FilterDiscovery struct {
	MinPrice   int
	MaxPrice   int
	Brands     []BrandFilterOption
	Quantities []QuantityFilterOption
	Attributes []AttributeFilter
}

// BrandFilterOption бренд з кількістю товарів
type BrandFilterOption struct {
	ID    uuid.UUID
	Name  string
	Slug  string
	Count int
}

// QuantityFilterOption фасовка з кількістю
type QuantityFilterOption struct {
	Value  float64
	Unit   string
	UnitID int
	Count  int
}

// AttributeFilter доступні значення для конкретної характеристики
type AttributeFilter struct {
	ID     int
	Code   string
	Name   string
	Values []AttrValueOption
}

// AttrValueOption значення характеристики з кількістю
type AttrValueOption struct {
	Code  string
	Label string
	Count int
}

// ImageUpload represents an image file to be uploaded to storage
type ImageUpload struct {
	Filename    string
	Content     interface{} // typically io.Reader, multipart.File or string path
	IsPrimary   bool
	IsHover     bool
	SortOrder   int
	VariationID *uuid.UUID // nil if product-level
	AltText     LocalizedMap
}

// QuickSearchProduct represents a lightweight product for autocomplete/search suggestions
type QuickSearchProduct struct {
	ID       uuid.UUID
	Name     string
	Slug     string
	SKU      string
	Price    int
	OldPrice *int
	ImageURL string
}

var (
	ErrBrandInUse    = errors.New("cannot delete brand: it is linked to existing products")
	ErrCategoryInUse = errors.New("cannot delete category: it is linked to existing products")
)

// PriceStrategy constants
const (
	PriceStrategyManual  = "manual"
	PriceStrategyDynamic = "dynamic"
)

// ProductRepository контракт для роботи з БД
type ProductRepository interface {
	FindAll(ctx context.Context, lang string, filter ProductFilter, pgn pagination.Params) ([]ProductVariation, int64, error)
	FindRecommended(ctx context.Context, lang string, limit int) ([]ProductVariation, error)
	FindByID(ctx context.Context, id uuid.UUID, lang string) (*Product, error)
	FindBySlug(ctx context.Context, slug string, lang string) (*Product, error)
	SlugExists(ctx context.Context, slug string, excludeID uuid.UUID) (bool, error)
	Exists(ctx context.Context, id uuid.UUID) (bool, error)
	FindReviews(ctx context.Context, productID uuid.UUID, pgn pagination.Params) ([]ProductReview, int64, error)
	FindPendingReviews(ctx context.Context, pgn pagination.Params) ([]ProductReview, int64, error)
	FindRejectedReviews(ctx context.Context, pgn pagination.Params) ([]ProductReview, int64, error)
	CreateReview(ctx context.Context, review *ProductReview) error
	ApproveReview(ctx context.Context, reviewID uuid.UUID) error
	RejectReview(ctx context.Context, reviewID uuid.UUID, reason *string) error
	ReopenReview(ctx context.Context, reviewID uuid.UUID) error
	GetFilters(ctx context.Context, filter ProductFilter, lang string) (*FilterDiscovery, error)
	QuickSearch(ctx context.Context, query string, lang string, limit int) ([]QuickSearchProduct, error)
	GetUnits(ctx context.Context) ([]Unit, error)

	// Admin CRUD
	Create(ctx context.Context, product *Product) error
	Update(ctx context.Context, product *Product) error
	Delete(ctx context.Context, id uuid.UUID) error
	CountByBrand(ctx context.Context, brandID uuid.UUID) (int64, error)
	CountByCategory(ctx context.Context, categoryID uuid.UUID) (int64, error)
	GetActiveCategoryIDs(ctx context.Context) (map[uuid.UUID]bool, error)

	// Bundle validation
	AreVariationsNonBundle(ctx context.Context, variationIDs []uuid.UUID) (bool, error)
	FindVariationsByIDs(ctx context.Context, ids []uuid.UUID) ([]ProductVariation, error)
	FindAttributeByCode(ctx context.Context, code string) (*Attribute, error)

	// Image Management
	FindImageByID(ctx context.Context, imageID uuid.UUID) (*ProductImage, error)
	DeleteImage(ctx context.Context, imageID uuid.UUID) error
	UpdateImage(ctx context.Context, image *ProductImage) error
	ReorderImages(ctx context.Context, productID uuid.UUID, ids []uuid.UUID) error
	CreateImages(ctx context.Context, images []ProductImage) error
	ResetImageRole(ctx context.Context, productID uuid.UUID, variationID *uuid.UUID, field string) error
}

// ProductService контракт для бізнес-логіки
type ProductService interface {
	GetList(ctx context.Context, lang string, filter ProductFilter, pgn pagination.Params) ([]ProductVariation, pagination.Metadata, error)
	GetRecommended(ctx context.Context, lang string, limit int) ([]ProductVariation, error)
	GetByID(ctx context.Context, id uuid.UUID, lang string) (*Product, error)
	GetBySlug(ctx context.Context, slug string, lang string) (*Product, error)
	GenerateSlug(ctx context.Context, name string, excludeID uuid.UUID) (string, error)
	GetReviews(ctx context.Context, productID uuid.UUID, pgn pagination.Params) ([]ProductReview, pagination.Metadata, error)
	GetPendingReviews(ctx context.Context, pgn pagination.Params) ([]ProductReview, pagination.Metadata, error)
	GetRejectedReviews(ctx context.Context, pgn pagination.Params) ([]ProductReview, pagination.Metadata, error)
	AddReview(ctx context.Context, review *ProductReview) error
	ApproveReview(ctx context.Context, reviewID uuid.UUID) error
	RejectReview(ctx context.Context, reviewID uuid.UUID, reason *string) error
	ReopenReview(ctx context.Context, reviewID uuid.UUID) error
	GetFilters(ctx context.Context, filter ProductFilter, lang string) (*FilterDiscovery, error)
	QuickSearch(ctx context.Context, query string, lang string, limit int) ([]QuickSearchProduct, error)
	GetUnits(ctx context.Context) ([]Unit, error)

	// Admin CRUD
	CreateProduct(ctx context.Context, product *Product, images []ImageUpload) error
	UpdateProduct(ctx context.Context, product *Product, variationsUpdate *[]ProductVariationUpdate, newImages []ImageUpload, imagesToDelete []uuid.UUID, isActive *bool, isBundle *bool, attributeValuesProvided bool) error
	DeleteProduct(ctx context.Context, id uuid.UUID) error

	// Image Management
	UploadProductImages(ctx context.Context, productID uuid.UUID, images []ImageUpload) ([]ProductImage, error)
	DeleteProductImage(ctx context.Context, productID uuid.UUID, imageID uuid.UUID) error
	UpdateProductImage(ctx context.Context, productID uuid.UUID, imageID uuid.UUID, isPrimary, isHover *bool, sortOrder *int, variationIDSet *bool, variationID *uuid.UUID, altTextUk, altTextEn *string) error
	ReorderProductImages(ctx context.Context, productID uuid.UUID, ids []uuid.UUID) error

	// General Media Upload
	UploadMedia(ctx context.Context, file interface{}, folder, filename string) (string, error)
}

// BrandRepository контракт для брендів
type BrandRepository interface {
	FindAll(ctx context.Context) ([]Brand, error)
	FindByID(ctx context.Context, id uuid.UUID) (*Brand, error)
	Create(ctx context.Context, brand *Brand) error
	Update(ctx context.Context, brand *Brand) error
	Delete(ctx context.Context, id uuid.UUID) error
	SlugExists(ctx context.Context, slug string) (bool, error)
}

// BrandService контракт для бізнес-логіки брендів
type BrandService interface {
	GetAll(ctx context.Context) ([]Brand, error)
	GetByID(ctx context.Context, id uuid.UUID) (*Brand, error)
	Create(ctx context.Context, brand *Brand) error
	Update(ctx context.Context, brand *Brand) error
	Delete(ctx context.Context, id uuid.UUID) error
	GenerateSlug(ctx context.Context, name string) (string, error)
}

// AttributeRepository контракт для атрибутів
type AttributeRepository interface {
	FindAll(ctx context.Context, lang string) ([]Attribute, error)
	FindByID(ctx context.Context, id int, lang string) (*Attribute, error)
	// FindValues повертає всі унікальні значення характеристики за її кодом,
	// незалежно від активності товарів (для підказок в адмінці).
	FindValues(ctx context.Context, attributeCode, lang string) ([]AttrValueOption, error)
	Create(ctx context.Context, attribute *Attribute) error
	Update(ctx context.Context, attribute *Attribute) error
	Delete(ctx context.Context, id int) error
	UpdateOrder(ctx context.Context, ids []int) error
}

// AttributeService контракт для бізнес-логіки атрибутів
type AttributeService interface {
	GetAll(ctx context.Context, lang string) ([]Attribute, error)
	GetByID(ctx context.Context, id int, lang string) (*Attribute, error)
	// GetValues повертає всі унікальні значення характеристики за її кодом
	// (напр. "type") — для підказок при створенні/редагуванні товару в адмінці.
	GetValues(ctx context.Context, attributeCode, lang string) ([]AttrValueOption, error)
	Create(ctx context.Context, attribute *Attribute) error
	Update(ctx context.Context, attribute *Attribute) error
	Delete(ctx context.Context, id int) error
	UpdateOrder(ctx context.Context, ids []int) error
}

// BadgeRepository контракт для бейджів
type BadgeRepository interface {
	FindAll(ctx context.Context) ([]Badge, error)
	FindByID(ctx context.Context, id int) (*Badge, error)
	Create(ctx context.Context, badge *Badge) error
	Update(ctx context.Context, badge *Badge) error
	Delete(ctx context.Context, id int) error
	CountUsage(ctx context.Context, badgeID int) (int64, error)
}

// BadgeService контракт для бізнес-логіки бейджів
type BadgeService interface {
	GetAll(ctx context.Context) ([]Badge, error)
	GetByID(ctx context.Context, id int) (*Badge, error)
	Create(ctx context.Context, badge *Badge) error
	Update(ctx context.Context, badge *Badge) error
	Delete(ctx context.Context, id int) error
}
