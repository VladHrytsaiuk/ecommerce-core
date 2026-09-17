package app

import (
	"fmt"

	"gorm.io/gorm"

	adminApp "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/application"
	adminDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/domain"
	adminPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/repository/postgres"
	badgesDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/badges/domain"
	catalogApp "github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/application"
	eventsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
	orderWorkflowApp "github.com/VladHrytsaiuk/ecommerce-core/internal/core/orderworkflow/application"
	mediaApp "github.com/VladHrytsaiuk/ecommerce-core/internal/media/application"
	ordersApp "github.com/VladHrytsaiuk/ecommerce-core/internal/orders/application"
	eventsPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/platform/postgres/events"
	reviewsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/reviews/domain"
	seoDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/seo/domain"
)

// adminFacades are the audited mutation boundaries. Each one pairs a change
// with an audit event in a single transaction, which is the condition the
// router states for exposing any permission-protected admin route.
type adminFacades struct {
	Catalog *adminApp.CatalogAdminFacade
	Orders  *adminApp.OrdersAdminFacade
	Content *adminApp.ContentAdminFacade
}

// adminFacadeDeps groups what the facades mutate. It exists so the signature
// names each collaborator rather than presenting a dozen positional arguments
// whose order a caller could silently transpose.
type adminFacadeDeps struct {
	Products         *catalogApp.ProductService
	Categories       *catalogApp.CategoryService
	Variants         *catalogApp.VariantService
	ProductOptions   *catalogApp.ProductOptionsService
	Inventory        adminApp.InventoryAdjuster
	MediaReader      *mediaApp.CatalogReader
	ProductPublisher eventsDomain.TransactionalEventPublisher
	Workflow         *orderWorkflowApp.Service
	WorkflowPolicy   *ordersApp.WorkflowService
	Badges           badgesDomain.Service
	Reviews          reviewsDomain.Service
	SEO              seoDomain.Service
}

func buildAdminFacades(db *gorm.DB, authorizer adminDomain.Authorizer, storeConfig StoreConfig, deps adminFacadeDeps) (adminFacades, error) {
	transactions := adminPostgres.NewTransactionManager(db)
	audit := func() eventsDomain.TransactionalEventPublisher {
		return eventsPostgres.NewPublisher(eventsDomain.ConsumerAdminAudit)
	}

	catalog, err := adminApp.NewCatalogAdminFacade(authorizer, deps.Products, deps.Categories, transactions, audit())
	if err != nil {
		return adminFacades{}, fmt.Errorf("configure admin catalog facade: %w", err)
	}
	catalog.WithProductEventPublisher(deps.ProductPublisher)
	catalog.WithProductOptions(deps.ProductOptions).WithInventoryAdjustment(deps.Inventory, storeConfig.DefaultWarehouseID)
	catalog.WithVariantMutations(deps.Variants)
	if deps.MediaReader != nil {
		catalog.WithMediaReader(deps.MediaReader)
	}

	orders, err := adminApp.NewOrdersAdminFacade(authorizer, deps.Workflow, transactions, audit())
	if err != nil {
		return adminFacades{}, fmt.Errorf("configure admin orders facade: %w", err)
	}
	if deps.WorkflowPolicy != nil {
		orders.WithStatusWorkflow(deps.WorkflowPolicy, deps.Workflow)
		orders.WithWorkflowConfiguration(deps.WorkflowPolicy)
	}

	// Badges, reviews and SEO were left unexposed until each mutation could
	// carry an audit event in its own transaction. The facade supplies that,
	// and attaches only the modules this deployment enabled.
	content, err := adminApp.NewContentAdminFacade(authorizer, transactions, audit())
	if err != nil {
		return adminFacades{}, fmt.Errorf("configure admin content facade: %w", err)
	}
	content.WithBadges(deps.Badges).WithReviews(deps.Reviews).WithSEO(deps.SEO)

	return adminFacades{Catalog: catalog, Orders: orders, Content: content}, nil
}
