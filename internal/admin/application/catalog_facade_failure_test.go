package application

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	catalogDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/domain"
)

// The admin audit log exists to answer "who changed what". It may never record
// a change that did not happen, so a refused mutation has to reach the caller
// and roll the transaction back before the event is written.
//
// It did not. `previous, err := reader.FindByIDForUpdate(...)` declared a
// second err scoped to the update branch; the Update call assigned to that one
// and the check after the if/else read the outer err, which was still nil. The
// facade returned success, published the audit event, and committed. The
// catalog services validate before touching the database, so a rejected update
// never aborts the transaction either — nothing downstream caught it.

var errCatalogRefused = errors.New("catalog refused the mutation")

func TestARefusedProductUpdateIsReportedAndNotAudited(t *testing.T) {
	transaction := &catalogSearchTransaction{}
	audit := &catalogEventPublisher{expected: transaction.marker}
	products := &refusingProducts{}
	facade, err := NewCatalogAdminFacade(catalogAllowAuthorizer{}, products, &refusingCategories{}, transaction, audit)
	if err != nil {
		t.Fatal(err)
	}

	err = facade.UpdateProduct(context.Background(), CatalogCommand{ActorUserID: uuid.New(), EventKey: uuid.New()}, validProduct())

	if !errors.Is(err, errCatalogRefused) {
		t.Fatalf("UpdateProduct() error = %v, want the repository's refusal", err)
	}
	if len(audit.events) != 0 {
		t.Fatalf("%d audit events recorded an update that was refused", len(audit.events))
	}
}

func TestARefusedCategoryUpdateIsReportedAndNotAudited(t *testing.T) {
	transaction := &catalogSearchTransaction{}
	audit := &catalogEventPublisher{expected: transaction.marker}
	categories := &refusingCategories{}
	facade, err := NewCatalogAdminFacade(catalogAllowAuthorizer{}, &refusingProducts{}, categories, transaction, audit)
	if err != nil {
		t.Fatal(err)
	}

	err = facade.UpdateCategory(context.Background(), CatalogCommand{ActorUserID: uuid.New(), EventKey: uuid.New()}, validCategory())

	if !errors.Is(err, errCatalogRefused) {
		t.Fatalf("UpdateCategory() error = %v, want the repository's refusal", err)
	}
	if len(audit.events) != 0 {
		t.Fatalf("%d audit events recorded an update that was refused", len(audit.events))
	}
}

func TestAnAcceptedUpdateIsStillAudited(t *testing.T) {
	// The correction must not turn every update into a failure.
	transaction := &catalogSearchTransaction{}
	audit := &catalogEventPublisher{expected: transaction.marker}
	products := &refusingProducts{accept: true}
	facade, err := NewCatalogAdminFacade(catalogAllowAuthorizer{}, products, &refusingCategories{}, transaction, audit)
	if err != nil {
		t.Fatal(err)
	}

	if err := facade.UpdateProduct(context.Background(), CatalogCommand{ActorUserID: uuid.New(), EventKey: uuid.New()}, validProduct()); err != nil {
		t.Fatalf("UpdateProduct() error = %v", err)
	}
	if len(audit.events) != 1 {
		t.Fatalf("audit events = %d, want exactly one", len(audit.events))
	}
}

func TestAMissingSnapshotStillStopsTheUpdate(t *testing.T) {
	// The lookup that read the previous state for the audit diff must keep
	// reporting its own failures after the rename.
	transaction := &catalogSearchTransaction{}
	audit := &catalogEventPublisher{expected: transaction.marker}
	products := &refusingProducts{accept: true, snapshotErr: errCatalogRefused}
	facade, err := NewCatalogAdminFacade(catalogAllowAuthorizer{}, products, &refusingCategories{}, transaction, audit)
	if err != nil {
		t.Fatal(err)
	}

	err = facade.UpdateProduct(context.Background(), CatalogCommand{ActorUserID: uuid.New(), EventKey: uuid.New()}, validProduct())

	if !errors.Is(err, errCatalogRefused) {
		t.Fatalf("UpdateProduct() error = %v, want the snapshot failure", err)
	}
	if len(audit.events) != 0 {
		t.Fatal("an audit event was written without the previous state it diffs against")
	}
}

func validProduct() *catalogDomain.Product {
	return &catalogDomain.Product{ID: uuid.New(), Status: "active",
		Translations: []catalogDomain.ProductTranslation{{Locale: "en", Name: "Headphones", Slug: "headphones"}}}
}

func validCategory() *catalogDomain.Category {
	return &catalogDomain.Category{ID: uuid.New(), IsActive: true,
		Translations: []catalogDomain.CategoryTranslation{{Locale: "en", Name: "Audio", Slug: "audio"}}}
}

type refusingProducts struct {
	accept      bool
	snapshotErr error
}

func (refusingProducts) Create(context.Context, *catalogDomain.Product) error { return nil }

func (p *refusingProducts) Update(context.Context, *catalogDomain.Product) error {
	if p.accept {
		return nil
	}
	return errCatalogRefused
}

func (p *refusingProducts) FindByIDForUpdate(context.Context, uuid.UUID) (*catalogDomain.Product, error) {
	if p.snapshotErr != nil {
		return nil, p.snapshotErr
	}
	return &catalogDomain.Product{ID: uuid.New(), Status: "active"}, nil
}

type refusingCategories struct{ accept bool }

func (refusingCategories) Create(context.Context, *catalogDomain.Category) error { return nil }

func (c *refusingCategories) Update(context.Context, *catalogDomain.Category) error {
	if c.accept {
		return nil
	}
	return errCatalogRefused
}

func (*refusingCategories) FindByIDForUpdate(context.Context, uuid.UUID) (*catalogDomain.Category, error) {
	return &catalogDomain.Category{ID: uuid.New(), IsActive: true}, nil
}
