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
			previous, err := reader.FindByIDForUpdate(tx, p.ID)
			if err != nil {
				return err
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
			previous, err := reader.FindByIDForUpdate(tx, c.ID)
			if err != nil {
				return err
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
