// Package app contains the clean-slate Composition Root.
package app

import (
	"context"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"gorm.io/gorm"

	catalogApp "github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/application"
	catalogDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/domain"
	catalogPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/repository/postgres"
	checkoutApp "github.com/VladHrytsaiuk/ecommerce-core/internal/checkout/application"
	checkoutDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/checkout/domain"
	localeApp "github.com/VladHrytsaiuk/ecommerce-core/internal/core/locale/application"
	localePostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/core/locale/repository/postgres"
	orderWorkflowApp "github.com/VladHrytsaiuk/ecommerce-core/internal/core/orderworkflow/application"
	orderWorkflowDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/core/orderworkflow/domain"
	orderWorkflowPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/core/orderworkflow/repository/postgres"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/tax"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware"
	inventoryApp "github.com/VladHrytsaiuk/ecommerce-core/internal/inventory/application"
	inventoryDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/inventory/domain"
	inventoryPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/inventory/repository/postgres"
	ordersApp "github.com/VladHrytsaiuk/ecommerce-core/internal/orders/application"
	ordersDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/orders/domain"
	ordersPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/orders/repository/postgres"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/config"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/security/token"
)

// Application exposes only services that belong to the active clean-slate graph.
type Application struct {
	Config                 *config.Config
	StoreConfig            StoreConfig
	TokenMaker             token.Maker
	CatalogCategoryService catalogDomain.CategoryService
	CatalogProductService  catalogDomain.ProductService
	CatalogVariantService  catalogDomain.VariantService
	CheckoutService        checkoutDomain.Service
	OrderWorkflowService   orderWorkflowDomain.Service
	InventoryService       inventoryDomain.Service
	OrderService           ordersDomain.Service
	TaxPolicy              tax.Calculator
	HTTP                   HTTPDependencies
}

type HTTPDependencies struct {
	Recovery         gin.HandlerFunc
	Timeout          gin.HandlerFunc
	RequestLogging   gin.HandlerFunc
	CORS             gin.HandlerFunc
	Swagger          gin.HandlerFunc
	Health           gin.HandlerFunc
	LocaleMiddleware gin.HandlerFunc
}

// Bootstrap is the sole Composition Root for the active clean-slate modules.
func Bootstrap(cfg *config.Config, storeConfig StoreConfig, db *gorm.DB, tokenMaker token.Maker) (*Application, error) {
	if err := storeConfig.Validate(); err != nil {
		return nil, err
	}
	taxPolicy, err := tax.NewPolicy(tax.Mode(storeConfig.TaxMode), storeConfig.VATRate)
	if err != nil {
		return nil, err
	}
	if err := localeApp.NewService(localePostgres.NewRepository(db)).Synchronize(context.Background(), storeConfig.SupportedLocales, storeConfig.DefaultLocale); err != nil {
		return nil, err
	}

	httpDependencies := HTTPDependencies{
		Recovery: gin.Recovery(),
		Timeout:  middleware.TimeoutMiddleware(cfg.RequestTimeout),
		RequestLogging: func(c *gin.Context) {
			logger.Log.Infow("Inbound Request", "method", c.Request.Method, "path", c.Request.URL.Path, "ip", c.ClientIP())
			c.Next()
		},
		CORS:             cors.New(cors.Config{AllowOrigins: cfg.CORSAllowOrigins, AllowMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}, AllowHeaders: []string{"Origin", "Content-Type", "Accept", "Authorization", "Accept-Language"}, AllowCredentials: true}),
		Swagger:          ginSwagger.WrapHandler(swaggerFiles.Handler),
		Health:           func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) },
		LocaleMiddleware: middleware.NewLocaleMiddleware(middleware.LocaleOptions{DefaultLocale: storeConfig.DefaultLocale, FallbackLocale: storeConfig.FallbackLocale, SupportedLocales: storeConfig.SupportedLocales}),
	}

	variantService := catalogApp.NewVariantService(catalogPostgres.NewVariantRepository(db), storeConfig.SupportedLocales, storeConfig.Currency)
	inventoryService := inventoryApp.NewService(inventoryMode(storeConfig.InventoryMode), inventoryPostgres.NewRepository(db))
	orderWorkflowService := orderWorkflowApp.NewService(orderWorkflowPostgres.NewRepository(db))

	return &Application{
		Config: cfg, StoreConfig: storeConfig, TokenMaker: tokenMaker,
		CatalogCategoryService: catalogApp.NewCategoryService(catalogPostgres.NewCategoryRepository(db), storeConfig.SupportedLocales),
		CatalogProductService:  catalogApp.NewProductService(catalogPostgres.NewProductRepository(db), storeConfig.SupportedLocales),
		CatalogVariantService:  variantService,
		CheckoutService:        checkoutApp.NewService(inventoryService, variantService, taxPolicy, checkoutDomain.Policy{AllowGuest: storeConfig.CheckoutAllowGuest, RequirePhone: storeConfig.CheckoutRequirePhone}, orderWorkflowService, nil),
		OrderWorkflowService:   orderWorkflowService,
		InventoryService:       inventoryService,
		OrderService:           ordersApp.NewService(ordersPostgres.NewRepository(db)),
		TaxPolicy:              taxPolicy,
		HTTP:                   httpDependencies,
	}, nil
}

func inventoryMode(value string) inventoryDomain.Mode {
	if value == "internal" {
		return inventoryDomain.ModeInternal
	}
	return inventoryDomain.ModeExternal
}
