package application

import (
	"context"
	"testing"

	"github.com/google/uuid"

	adminDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/domain"
	catalogDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
)

func TestCatalogFacadePublishesSearchChangeInMutationTransaction(t *testing.T) {
	actor, productID, eventKey := uuid.New(), uuid.New(), uuid.New()
	tx := &catalogSearchTransaction{}
	auditPublisher := &catalogEventPublisher{expected: tx.marker}
	searchPublisher := &catalogEventPublisher{expected: tx.marker}
	products := &catalogSearchProducts{}
	facade, err := NewCatalogAdminFacade(catalogAllowAuthorizer{}, products, catalogSearchCategories{}, tx, auditPublisher)
	if err != nil {
		t.Fatal(err)
	}
	facade.WithProductEventPublisher(searchPublisher)
	product := &catalogDomain.Product{ID: productID, Status: "active", Translations: []catalogDomain.ProductTranslation{{Locale: "en", Name: "Headphones", Slug: "headphones"}}}
	if err := facade.CreateProduct(context.Background(), CatalogCommand{ActorUserID: actor, EventKey: eventKey}, product); err != nil {
		t.Fatalf("CreateProduct() error = %v", err)
	}
	if !products.created || len(auditPublisher.events) != 1 || len(searchPublisher.events) != 1 {
		t.Fatalf("mutation/audit/search = %v/%d/%d", products.created, len(auditPublisher.events), len(searchPublisher.events))
	}
	if got := searchPublisher.events[0].Topic(); got != catalogDomain.TopicProductChanged {
		t.Fatalf("search event topic = %q", got)
	}
}

type transactionMarker struct{}

type catalogSearchTransaction struct{ marker transactionMarker }

func (t *catalogSearchTransaction) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(context.WithValue(ctx, t.marker, "catalog-tx"))
}

type catalogAllowAuthorizer struct{}

func (catalogAllowAuthorizer) Require(context.Context, uuid.UUID, string) error { return nil }

type catalogSearchProducts struct{ created bool }

func (p *catalogSearchProducts) Create(context.Context, *catalogDomain.Product) error {
	p.created = true
	return nil
}
func (*catalogSearchProducts) Update(context.Context, *catalogDomain.Product) error { return nil }

type catalogSearchCategories struct{}

func (catalogSearchCategories) Create(context.Context, *catalogDomain.Category) error { return nil }
func (catalogSearchCategories) Update(context.Context, *catalogDomain.Category) error { return nil }

type catalogEventPublisher struct {
	expected transactionMarker
	events   []events.DomainEvent
}

func (p *catalogEventPublisher) Publish(ctx context.Context, event events.DomainEvent) error {
	if ctx.Value(p.expected) != "catalog-tx" {
		return adminDomain.ErrPermissionDenied
	}
	p.events = append(p.events, event)
	return nil
}
