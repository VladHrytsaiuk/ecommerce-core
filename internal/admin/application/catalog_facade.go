package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	adminDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
	"github.com/google/uuid"
)

const PermissionCatalogWrite = "catalog:write"

// ErrMediaAssetsNotReady is safe for HTTP delivery to expose as a bad request;
// the wrapped storage detail remains available only to structured logs.
var ErrMediaAssetsNotReady = errors.New("catalog media assets are not ready")

type CatalogProducts interface {
	Create(context.Context, *domain.Product) error
	Update(context.Context, *domain.Product) error
}
type CatalogProductDeleter interface {
	Delete(context.Context, uuid.UUID) error
}
type CatalogProductSnapshots interface {
	FindByIDForUpdate(context.Context, uuid.UUID) (*domain.Product, error)
}
type CatalogProductOptions interface {
	CreateProductOption(context.Context, *domain.ProductOption) error
	CreateVariant(context.Context, *domain.ProductVariant, []uuid.UUID) error
}

// CatalogVariants is the mutation surface for an existing sellable unit. It is
// separate from CatalogProductOptions because creating a variant needs the
// option-value wiring while editing one does not.
type CatalogVariants interface {
	FindVariantForUpdate(context.Context, uuid.UUID) (*domain.ProductVariant, error)
	Update(context.Context, uuid.UUID, domain.UpdateVariantCommand) (*domain.ProductVariant, error)
	Archive(context.Context, uuid.UUID) (*domain.ProductVariant, error)
}
type InventoryAdjuster interface {
	Adjust(context.Context, uuid.UUID, uuid.UUID, int) error
}
type CatalogCategories interface {
	Create(context.Context, *domain.Category) error
	Update(context.Context, *domain.Category) error
}
type CatalogCategorySnapshots interface {
	FindByIDForUpdate(context.Context, uuid.UUID) (*domain.Category, error)
}

type CatalogAdminFacade struct {
	authorizer    adminDomain.Authorizer
	products      CatalogProducts
	categories    CatalogCategories
	tx            TransactionManager
	publisher     events.TransactionalEventPublisher
	productEvents events.TransactionalEventPublisher
	mediaReader   domain.MediaReader
	options       CatalogProductOptions
	variants      CatalogVariants
	inventory     InventoryAdjuster
	warehouseID   uuid.UUID
}

func (f *CatalogAdminFacade) WithProductOptions(service CatalogProductOptions) *CatalogAdminFacade {
	if f != nil {
		f.options = service
	}
	return f
}

// WithInventoryAdjustment supplies the configured default warehouse only at
// composition time. The HTTP boundary never accepts a warehouse identifier.
func (f *CatalogAdminFacade) WithInventoryAdjustment(service InventoryAdjuster, warehouseID uuid.UUID) *CatalogAdminFacade {
	if f != nil {
		f.inventory, f.warehouseID = service, warehouseID
	}
	return f
}

// WithVariantMutations enables editing and archiving existing variants.
func (f *CatalogAdminFacade) WithVariantMutations(service CatalogVariants) *CatalogAdminFacade {
	if f != nil {
		f.variants = service
	}
	return f
}

func (f *CatalogAdminFacade) HasVariantMutations() bool { return f != nil && f.variants != nil }

// UpdateVariant edits an existing sellable unit.
//
// The prior state is read under a write lock inside the transaction, so the
// audit entry records what was actually replaced — a price change is the kind
// of edit whose previous value matters most after the fact.
func (f *CatalogAdminFacade) UpdateVariant(ctx context.Context, cmd CatalogCommand, variantID uuid.UUID, command domain.UpdateVariantCommand) (*domain.ProductVariant, error) {
	if err := f.authorizeVariant(ctx, cmd); err != nil {
		return nil, err
	}
	if variantID == uuid.Nil {
		return nil, fmt.Errorf("catalog variant ID is required")
	}
	var updated *domain.ProductVariant
	err := f.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		previous, err := f.variants.FindVariantForUpdate(txCtx, variantID)
		if err != nil {
			return err
		}
		variant, err := f.variants.Update(txCtx, variantID, command)
		if err != nil {
			return err
		}
		updated = variant
		if err := f.auditVariant(txCtx, cmd, "catalog.variant.update", variantID, previous, variant); err != nil {
			return err
		}
		// The search projection indexes the product, not the variant, so a
		// price or status edit has to re-index its parent.
		return f.publishProductChanged(txCtx, cmd.EventKey, variant.ProductID, domain.ProductChangeUpsert)
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

// ArchiveVariant withdraws a variant from sale. It is deliberately not a row
// deletion; see ArchiveVariant on the catalog repository port for why.
func (f *CatalogAdminFacade) ArchiveVariant(ctx context.Context, cmd CatalogCommand, variantID uuid.UUID) (*domain.ProductVariant, error) {
	if err := f.authorizeVariant(ctx, cmd); err != nil {
		return nil, err
	}
	if variantID == uuid.Nil {
		return nil, fmt.Errorf("catalog variant ID is required")
	}
	var archived *domain.ProductVariant
	err := f.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		previous, err := f.variants.FindVariantForUpdate(txCtx, variantID)
		if err != nil {
			return err
		}
		variant, err := f.variants.Archive(txCtx, variantID)
		if err != nil {
			return err
		}
		archived = variant
		if err := f.auditVariant(txCtx, cmd, "catalog.variant.archive", variantID, previous, variant); err != nil {
			return err
		}
		return f.publishProductChanged(txCtx, cmd.EventKey, variant.ProductID, domain.ProductChangeUpsert)
	})
	if err != nil {
		return nil, err
	}
	return archived, nil
}

func (f *CatalogAdminFacade) authorizeVariant(ctx context.Context, cmd CatalogCommand) error {
	if f == nil || f.variants == nil {
		return fmt.Errorf("catalog variant mutations are not configured")
	}
	if cmd.ActorUserID == uuid.Nil {
		return adminDomain.ErrNotAdmin
	}
	return f.authorizer.Require(ctx, cmd.ActorUserID, PermissionCatalogWrite)
}

func (f *CatalogAdminFacade) auditVariant(ctx context.Context, cmd CatalogCommand, action string, variantID uuid.UUID, previous, current *domain.ProductVariant) error {
	old, err := json.Marshal(previous)
	if err != nil {
		return fmt.Errorf("encode %s previous state: %w", action, err)
	}
	updated, err := json.Marshal(current)
	if err != nil {
		return fmt.Errorf("encode %s new state: %w", action, err)
	}
	if cmd.EventKey == uuid.Nil {
		cmd.EventKey = uuid.New()
	}
	event, err := adminDomain.NewAdminActionEvent(cmd.EventKey, cmd.ActorUserID, action, "product_variant", variantID, old, updated, cmd.IPAddress, nil, nowUTC())
	if err != nil {
		return err
	}
	return f.publisher.Publish(ctx, event)
}

func (f *CatalogAdminFacade) WithMediaReader(r domain.MediaReader) *CatalogAdminFacade {
	f.mediaReader = r
	return f
}

// WithProductEventPublisher attaches the optional Search projection publisher.
// It is selected only by Bootstrap when the Search module is enabled.
func (f *CatalogAdminFacade) WithProductEventPublisher(publisher events.TransactionalEventPublisher) *CatalogAdminFacade {
	if f != nil {
		f.productEvents = publisher
	}
	return f
}

func NewCatalogAdminFacade(a adminDomain.Authorizer, p CatalogProducts, c CatalogCategories, tx TransactionManager, publisher events.TransactionalEventPublisher) (*CatalogAdminFacade, error) {
	if a == nil || p == nil || c == nil || tx == nil || publisher == nil {
		return nil, fmt.Errorf("catalog admin facade is not configured")
	}
	return &CatalogAdminFacade{authorizer: a, products: p, categories: c, tx: tx, publisher: publisher}, nil
}

type CatalogCommand struct {
	ActorUserID, EventKey uuid.UUID
	IPAddress             string
}

func (f *CatalogAdminFacade) CreateProduct(ctx context.Context, cmd CatalogCommand, product *domain.Product) error {
	return f.product(ctx, cmd, product, false)
}
func (f *CatalogAdminFacade) UpdateProduct(ctx context.Context, cmd CatalogCommand, product *domain.Product) error {
	return f.product(ctx, cmd, product, true)
}
func (f *CatalogAdminFacade) product(ctx context.Context, cmd CatalogCommand, p *domain.Product, update bool) error {
	if err := f.authorizer.Require(ctx, cmd.ActorUserID, PermissionCatalogWrite); err != nil {
		return err
	}
	if cmd.EventKey == uuid.Nil {
		cmd.EventKey = uuid.New()
	}
	return f.tx.WithinTransaction(ctx, func(tx context.Context) error {
		if len(p.Media) > 0 {
			if f.mediaReader == nil {
				return fmt.Errorf("media module is not configured")
			}
			ids := make([]uuid.UUID, 0, len(p.Media))
			for _, m := range p.Media {
				ids = append(ids, m.AssetID)
			}
			if err := f.mediaReader.CheckAssetsReady(tx, ids); err != nil {
				return fmt.Errorf("%w: %v", ErrMediaAssetsNotReady, err)
			}
		}
		var err error
		action := "catalog.product.create"
		var old []byte
		if update {
			reader, ok := f.products.(CatalogProductSnapshots)
			if !ok {
				return fmt.Errorf("catalog product snapshot reader is not configured")
			}
			// findErr, not err: `previous, err :=` declares a second err
			// scoped to this block, so the Update below assigned to that one
			// and the check after the if/else read the outer err, still nil.
			// A refused update returned success and was audited as done.
			previous, findErr := reader.FindByIDForUpdate(tx, p.ID)
			if findErr != nil {
				return findErr
			}
			old, _ = json.Marshal(previous)
			action = "catalog.product.update"
			err = f.products.Update(tx, p)
		} else {
			err = f.products.Create(tx, p)
		}
		if err != nil {
			return err
		}
		raw, _ := json.Marshal(p)
		e, err := adminDomain.NewAdminActionEvent(cmd.EventKey, cmd.ActorUserID, action, "product", p.ID, old, raw, cmd.IPAddress, nil, nowUTC())
		if err != nil {
			return err
		}
		if err := f.publisher.Publish(tx, e); err != nil {
			return err
		}
		return f.publishProductChanged(tx, cmd.EventKey, p.ID, domain.ProductChangeUpsert)
	})
}

func (f *CatalogAdminFacade) DeleteProduct(ctx context.Context, cmd CatalogCommand, productID uuid.UUID) error {
	if err := f.authorizer.Require(ctx, cmd.ActorUserID, PermissionCatalogWrite); err != nil {
		return err
	}
	if productID == uuid.Nil {
		return fmt.Errorf("catalog product ID is required")
	}
	if cmd.EventKey == uuid.Nil {
		cmd.EventKey = uuid.New()
	}
	return f.tx.WithinTransaction(ctx, func(tx context.Context) error {
		reader, ok := f.products.(CatalogProductSnapshots)
		if !ok {
			return fmt.Errorf("catalog product snapshot reader is not configured")
		}
		previous, err := reader.FindByIDForUpdate(tx, productID)
		if err != nil {
			return err
		}
		deleter, ok := f.products.(CatalogProductDeleter)
		if !ok {
			return fmt.Errorf("catalog product delete is not configured")
		}
		if err := deleter.Delete(tx, productID); err != nil {
			return err
		}
		old, _ := json.Marshal(previous)
		e, err := adminDomain.NewAdminActionEvent(cmd.EventKey, cmd.ActorUserID, "catalog.product.delete", "product", productID, old, nil, cmd.IPAddress, nil, nowUTC())
		if err != nil {
			return err
		}
		if err := f.publisher.Publish(tx, e); err != nil {
			return err
		}
		return f.publishProductChanged(tx, cmd.EventKey, productID, domain.ProductChangeDelete)
	})
}

func (f *CatalogAdminFacade) publishProductChanged(ctx context.Context, eventKey, productID uuid.UUID, operation domain.ProductChangeOperation) error {
	if f.productEvents == nil {
		return nil
	}
	event, err := domain.NewProductChangedEvent(eventKey, productID, operation, nowUTC())
	if err != nil {
		return err
	}
	return f.productEvents.Publish(ctx, event)
}

func (f *CatalogAdminFacade) CreateProductOption(ctx context.Context, cmd CatalogCommand, productID uuid.UUID, option *domain.ProductOption) error {
	if err := f.authorizer.Require(ctx, cmd.ActorUserID, PermissionCatalogWrite); err != nil {
		return err
	}
	if f.options == nil || productID == uuid.Nil || option == nil {
		return fmt.Errorf("catalog options are not configured")
	}
	if cmd.EventKey == uuid.Nil {
		cmd.EventKey = uuid.New()
	}
	return f.tx.WithinTransaction(ctx, func(tx context.Context) error {
		reader, ok := f.products.(CatalogProductSnapshots)
		if !ok {
			return fmt.Errorf("catalog product snapshot reader is not configured")
		}
		if _, err := reader.FindByIDForUpdate(tx, productID); err != nil {
			return err
		}
		option.ProductID = productID
		if err := f.options.CreateProductOption(tx, option); err != nil {
			return err
		}
		raw, err := json.Marshal(option)
		if err != nil {
			return err
		}
		e, err := adminDomain.NewAdminActionEvent(cmd.EventKey, cmd.ActorUserID, "catalog.product_option.create", "product_option", option.ID, nil, raw, cmd.IPAddress, nil, nowUTC())
		if err != nil {
			return err
		}
		if err := f.publisher.Publish(tx, e); err != nil {
			return err
		}
		return f.publishProductChanged(tx, cmd.EventKey, productID, domain.ProductChangeUpsert)
	})
}

func (f *CatalogAdminFacade) CreateProductVariant(ctx context.Context, cmd CatalogCommand, productID uuid.UUID, variant *domain.ProductVariant, optionValueIDs []uuid.UUID, initialQuantity int) error {
	if err := f.authorizer.Require(ctx, cmd.ActorUserID, PermissionCatalogWrite); err != nil {
		return err
	}
	if f.options == nil || productID == uuid.Nil || variant == nil || initialQuantity < 0 {
		return fmt.Errorf("catalog variant request is invalid")
	}
	if initialQuantity > 0 && (f.inventory == nil || f.warehouseID == uuid.Nil) {
		return fmt.Errorf("inventory adjustment is not configured")
	}
	if cmd.EventKey == uuid.Nil {
		cmd.EventKey = uuid.New()
	}
	return f.tx.WithinTransaction(ctx, func(tx context.Context) error {
		reader, ok := f.products.(CatalogProductSnapshots)
		if !ok {
			return fmt.Errorf("catalog product snapshot reader is not configured")
		}
		if _, err := reader.FindByIDForUpdate(tx, productID); err != nil {
			return err
		}
		variant.ProductID = productID
		if err := f.options.CreateVariant(tx, variant, optionValueIDs); err != nil {
			return err
		}
		if initialQuantity > 0 {
			if err := f.inventory.Adjust(tx, variant.ID, f.warehouseID, initialQuantity); err != nil {
				return err
			}
		}
		raw, err := json.Marshal(struct {
			Variant         *domain.ProductVariant `json:"variant"`
			InitialQuantity int                    `json:"initial_quantity"`
		}{variant, initialQuantity})
		if err != nil {
			return err
		}
		e, err := adminDomain.NewAdminActionEvent(cmd.EventKey, cmd.ActorUserID, "catalog.product_variant.create", "product_variant", variant.ID, nil, raw, cmd.IPAddress, nil, nowUTC())
		if err != nil {
			return err
		}
		if err := f.publisher.Publish(tx, e); err != nil {
			return err
		}
		return f.publishProductChanged(tx, cmd.EventKey, productID, domain.ProductChangeUpsert)
	})
}
func (f *CatalogAdminFacade) CreateCategory(ctx context.Context, cmd CatalogCommand, c *domain.Category) error {
	return f.category(ctx, cmd, c, false)
}
func (f *CatalogAdminFacade) UpdateCategory(ctx context.Context, cmd CatalogCommand, c *domain.Category) error {
	return f.category(ctx, cmd, c, true)
}
func (f *CatalogAdminFacade) category(ctx context.Context, cmd CatalogCommand, c *domain.Category, update bool) error {
	if err := f.authorizer.Require(ctx, cmd.ActorUserID, PermissionCatalogWrite); err != nil {
		return err
	}
	if cmd.EventKey == uuid.Nil {
		cmd.EventKey = uuid.New()
	}
	return f.tx.WithinTransaction(ctx, func(tx context.Context) error {
		var err error
		action := "catalog.category.create"
		var old []byte
		if update {
			reader, ok := f.categories.(CatalogCategorySnapshots)
			if !ok {
				return fmt.Errorf("catalog category snapshot reader is not configured")
			}
			// findErr, not err: see the same correction in the product path.
			previous, findErr := reader.FindByIDForUpdate(tx, c.ID)
			if findErr != nil {
				return findErr
			}
			old, _ = json.Marshal(previous)
			action = "catalog.category.update"
			err = f.categories.Update(tx, c)
		} else {
			err = f.categories.Create(tx, c)
		}
		if err != nil {
			return err
		}
		raw, _ := json.Marshal(c)
		e, err := adminDomain.NewAdminActionEvent(cmd.EventKey, cmd.ActorUserID, action, "category", c.ID, old, raw, cmd.IPAddress, nil, nowUTC())
		if err != nil {
			return err
		}
		return f.publisher.Publish(tx, e)
	})
}
