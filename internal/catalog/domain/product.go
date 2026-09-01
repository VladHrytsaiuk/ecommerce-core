package domain

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrProductNotFound = errors.New("product not found")
	ErrInvalidProduct  = errors.New("invalid product")
)

// Product is the clean catalog aggregate. Localized business content belongs
// only to ProductTranslation records, never to JSONB fields on the product.
type Product struct {
	ID           uuid.UUID            `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	CategoryID   *uuid.UUID           `gorm:"type:uuid" json:"category_id,omitempty"`
	Status       string               `gorm:"type:varchar(32);not null" json:"status"`
	CreatedAt    time.Time            `json:"created_at"`
	UpdatedAt    time.Time            `json:"updated_at"`
	Translations []ProductTranslation `gorm:"foreignKey:ProductID;constraint:OnDelete:CASCADE" json:"translations"`
	Rating       *ProductRating       `gorm:"-" json:"rating,omitempty"`
	SEO          *ProductSEO          `gorm:"-" json:"seo,omitempty"`
	Badges       []ProductBadge       `gorm:"-" json:"badges,omitempty"`
	Media        []ProductMedia       `gorm:"foreignKey:ProductID" json:"media,omitempty"`
	// Options are loaded explicitly by the product-options read port. They are
	// not GORM-preloaded by the base product repository to keep existing reads
	// backward-compatible and avoid an accidental N+1 query.
	Options  []ProductOption  `gorm:"-" json:"options,omitempty"`
	Variants []ProductVariant `gorm:"-" json:"variants,omitempty"`
}

// ProductOption is a merchant-defined dimension of a product, such as Color,
// Size or Volume. Its values compose a ProductVariant; it never replaces the
// variant as the sellable and inventory-tracked unit.
type ProductOption struct {
	ID        uuid.UUID            `json:"id"`
	ProductID uuid.UUID            `json:"product_id"`
	Name      string               `json:"name"`
	Position  int                  `json:"position"`
	Values    []ProductOptionValue `gorm:"-" json:"values,omitempty"`
}

// ProductOptionValue is one value within an option, for example Red or XL.
// ProductID is retained in the DTO to make ownership explicit at the module
// boundary; persistence additionally enforces it with composite foreign keys.
type ProductOptionValue struct {
	ID        uuid.UUID `json:"id"`
	OptionID  uuid.UUID `json:"option_id"`
	ProductID uuid.UUID `json:"product_id"`
	Value     string    `json:"value"`
	Position  int       `json:"position"`
}
type ProductMedia struct {
	ProductID uuid.UUID  `json:"product_id"`
	VariantID *uuid.UUID `json:"variant_id,omitempty"`
	AssetID   uuid.UUID  `json:"asset_id"`
	Role      string     `json:"role"`
	Position  int        `json:"position"`
	URL       string     `gorm:"-" json:"url,omitempty"`
}

func (ProductMedia) TableName() string { return "product_media" }

type MediaReader interface {
	CheckAssetsReady(context.Context, []uuid.UUID) error
	GetPublicURLs(context.Context, []uuid.UUID) (map[uuid.UUID]string, error)
}

// ProductRating is a read projection owned by the optional Reviews module;
// it is deliberately not persisted as a column on Core products.
type ProductRating struct {
	ReviewCount       int `json:"review_count"`
	AverageHundredths int `json:"average_hundredths"`
}

// ProductRatingReader is a Catalog-owned port. The Reviews PostgreSQL adapter
// implements it only when the module is enabled and Bootstrap wires it in.
type ProductRatingReader interface {
	RatingForProduct(context.Context, uuid.UUID) (*ProductRating, error)
	RatingsForProducts(context.Context, []uuid.UUID) (map[uuid.UUID]ProductRating, error)
}

// ProductSEO and ProductBadge are optional read models. Catalog owns their
// ports so it can be enriched without importing the optional modules.
type ProductSEO struct {
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Keywords    string `json:"keywords,omitempty"`
	OGImageRef  string `json:"og_image_ref,omitempty"`
}

type ProductSEOReader interface {
	SEOForResources(context.Context, string, []uuid.UUID, string) (map[uuid.UUID]ProductSEO, error)
}

type ProductBadge struct {
	ID    uuid.UUID `json:"id"`
	Slug  string    `json:"slug"`
	Color string    `json:"color"`
	Name  string    `json:"name"`
}

type ProductBadgeReader interface {
	BadgesForProducts(context.Context, []uuid.UUID, string) (map[uuid.UUID][]ProductBadge, error)
}

func (Product) TableName() string { return "products" }

type ProductTranslation struct {
	ID          uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	ProductID   uuid.UUID `gorm:"type:uuid;not null;index" json:"product_id"`
	Locale      string    `gorm:"type:varchar(10);not null" json:"locale"`
	Name        string    `gorm:"type:text;not null" json:"name"`
	Description string    `gorm:"type:text" json:"description,omitempty"`
	Slug        string    `gorm:"type:varchar(255);not null" json:"slug"`
}

func (ProductTranslation) TableName() string { return "product_translations" }

type ProductRepository interface {
	FindBySlug(context.Context, string, string) (*Product, error)
	List(context.Context) ([]Product, error)
	ListProducts(context.Context, string, int, int) ([]Product, int64, error)
	Create(context.Context, *Product) error
}

// ProductSnapshotRepository is a narrow read port for asynchronous product
// projections. Existing Catalog command/query ports remain unchanged.
type ProductSnapshotRepository interface {
	FindByID(context.Context, uuid.UUID) (*Product, error)
}

// ProductOptionsRepository owns option-matrix writes. Implementations must
// keep every write for one product in a single transaction.
type ProductOptionsRepository interface {
	CreateProductOption(context.Context, *ProductOption) error
	CreateVariantWithOptionValues(context.Context, *ProductVariant, []uuid.UUID) error
}

// ProductOptionsService is the narrow command port used by the audited Admin
// facade. It deliberately has no HTTP or persistence dependencies.
type ProductOptionsService interface {
	CreateProductOption(context.Context, *ProductOption) error
	CreateVariant(context.Context, *ProductVariant, []uuid.UUID) error
}

// VariantAvailabilityReader is a Catalog-owned read port. Inventory adapters
// may implement it, but availability never becomes a client-provided field.
type VariantAvailabilityReader interface {
	AvailabilityForVariants(context.Context, []uuid.UUID) (map[uuid.UUID]bool, error)
}

type ActiveProductRepository interface {
	ListActiveAfter(context.Context, *uuid.UUID, int) ([]Product, error)
}

// ProductService is the application-facing port used by delivery adapters.
// It deliberately exposes translation lists instead of language-specific
// product fields.
type ProductService interface {
	FindBySlug(context.Context, string, string) (*Product, error)
	List(context.Context, string) ([]Product, error)
	ListProducts(context.Context, string, int, int) ([]Product, int64, error)
	Create(context.Context, *Product) error
}

type ProductSnapshotService interface {
	FindByID(context.Context, uuid.UUID) (*Product, error)
}

type ActiveProductService interface {
	ListActiveAfter(context.Context, *uuid.UUID, int) ([]Product, error)
}

type AdminProductRepository interface {
	Update(context.Context, *Product) error
	Delete(context.Context, uuid.UUID) error
}
