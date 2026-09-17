package app

import (
	"fmt"

	"gorm.io/gorm"

	deliveryWorkflowAdapter "github.com/VladHrytsaiuk/ecommerce-core/internal/adapters/delivery/orderworkflow"
	checkoutDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/checkout/domain"
	orderWorkflowApp "github.com/VladHrytsaiuk/ecommerce-core/internal/core/orderworkflow/application"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/tax"
	ordersApp "github.com/VladHrytsaiuk/ecommerce-core/internal/orders/application"
	ordersPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/orders/repository/postgres"
	paymentsApp "github.com/VladHrytsaiuk/ecommerce-core/internal/payments/application"
	paymentsPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/payments/repository/postgres"
	eventsPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/platform/postgres/events"
	workflowPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/platform/postgres/orderworkflow"
	promosApp "github.com/VladHrytsaiuk/ecommerce-core/internal/promos/application"
	promosPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/promos/repository/postgres"
)

// commerceRuntime is the order workflow and everything that decorates it.
//
// Unlike the optional modules, this is assembled in one place because the
// order in which its parts attach is load-bearing: the promos hook and the
// operational transition policy are registered on the workflow repository
// before any service wraps it, so every caller sees the same configured
// repository rather than a partially decorated one.
type commerceRuntime struct {
	Workflow        *orderWorkflowApp.Service
	Policy          *ordersApp.WorkflowService
	PriceCalculator checkoutDomain.PriceCalculator
	Promos          *promosPostgres.Repository
	PaymentWebhooks *paymentsApp.WebhookService
	DeliveryBridge  *deliveryWorkflowAdapter.Bridge
}

func buildCommerce(db *gorm.DB, modules ModuleSet, taxPolicy tax.Calculator, gateways *paymentsApp.Registry, eventRoutes map[string][]string) (commerceRuntime, error) {
	publisher := eventsPostgres.NewTopicPublisher(eventRoutes)
	repository := workflowPostgres.NewRepository(db, modules.Has(ModuleSync)).
		WithEventPublisher(publisher).
		// The status lifecycle is already durably recorded in
		// order_status_history. Persist its public event separately until a
		// module explicitly subscribes, so payment/report workers never claim
		// unrelated status deliveries.
		WithStatusEventPublisher(publisher)

	var runtime commerceRuntime
	if modules.Has(ModuleOrders) {
		policy, err := ordersApp.NewWorkflowService(ordersPostgres.NewWorkflowRepository(db))
		if err != nil {
			return commerceRuntime{}, fmt.Errorf("configure order workflow policy: %w", err)
		}
		runtime.Policy = policy
		repository.WithOperationalTransitionPolicy(policy)
	}

	prices, err := checkoutDomain.NewCheckoutPriceCalculator(taxPolicy)
	if err != nil {
		return commerceRuntime{}, err
	}
	runtime.PriceCalculator = prices
	if modules.Has(ModulePromos) {
		runtime.Promos = promosPostgres.NewRepository(db)
		// The hook must be attached before the workflow service wraps the
		// repository: a redemption is reserved in the same transaction that
		// creates the pending order.
		repository.WithTransactionHook(promosApp.NewWorkflowHook(runtime.Promos))
		runtime.PriceCalculator = promosApp.NewPromoCalculatorDecorator(runtime.PriceCalculator, runtime.Promos)
	}

	runtime.Workflow = orderWorkflowApp.NewService(repository)
	if runtime.Policy != nil {
		bridge, bridgeErr := deliveryWorkflowAdapter.NewBridge(runtime.Workflow)
		if bridgeErr != nil {
			return commerceRuntime{}, fmt.Errorf("configure delivery order workflow bridge: %w", bridgeErr)
		}
		runtime.DeliveryBridge = bridge
	}
	runtime.PaymentWebhooks = paymentsApp.NewWebhookService(gateways, paymentsPostgres.NewWebhookEventStore(db), runtime.Workflow)
	return runtime, nil
}
