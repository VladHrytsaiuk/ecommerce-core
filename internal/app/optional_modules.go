package app

import (
	"context"
	"fmt"
	"io"
	"time"

	"gorm.io/gorm"

	abandonedReaders "github.com/VladHrytsaiuk/ecommerce-core/internal/abandoned_cart/adapter/readers"
	abandonedApp "github.com/VladHrytsaiuk/ecommerce-core/internal/abandoned_cart/application"
	abandonedPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/abandoned_cart/repository/postgres"
	googleAuthAdapter "github.com/VladHrytsaiuk/ecommerce-core/internal/adapters/auth/google"
	adminDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/domain"
	adminPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/repository/postgres"
	availabilityIdentity "github.com/VladHrytsaiuk/ecommerce-core/internal/availability_notifications/adapter/identity"
	availabilityApp "github.com/VladHrytsaiuk/ecommerce-core/internal/availability_notifications/application"
	availabilityDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/availability_notifications/domain"
	availabilityPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/availability_notifications/repository/postgres"
	cartApp "github.com/VladHrytsaiuk/ecommerce-core/internal/cart/application"
	catalogApp "github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/application"
	checkoutConsent "github.com/VladHrytsaiuk/ecommerce-core/internal/checkout/adapter/consent"
	checkoutApp "github.com/VladHrytsaiuk/ecommerce-core/internal/checkout/application"
	checkoutDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/checkout/domain"
	checkoutPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/checkout/repository/postgres"
	consentOrders "github.com/VladHrytsaiuk/ecommerce-core/internal/consent/adapter/orders"
	consentUnsubscribe "github.com/VladHrytsaiuk/ecommerce-core/internal/consent/adapter/unsubscribe"
	consentApp "github.com/VladHrytsaiuk/ecommerce-core/internal/consent/application"
	consentDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/consent/domain"
	consentPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/consent/repository/postgres"
	eventsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
	eventsApp "github.com/VladHrytsaiuk/ecommerce-core/internal/core/events/application"
	identityNotifications "github.com/VladHrytsaiuk/ecommerce-core/internal/identity/adapter/notifications"
	identityApplication "github.com/VladHrytsaiuk/ecommerce-core/internal/identity/application"
	identityDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/identity/domain"
	identityPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/identity/repository/postgres"
	identityService "github.com/VladHrytsaiuk/ecommerce-core/internal/identity/service"
	inventoryApp "github.com/VladHrytsaiuk/ecommerce-core/internal/inventory/application"
	mediaApp "github.com/VladHrytsaiuk/ecommerce-core/internal/media/application"
	mediaPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/media/repository/postgres"
	notificationsApp "github.com/VladHrytsaiuk/ecommerce-core/internal/notifications/application"
	notificationsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/notifications/domain"
	notificationsPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/notifications/repository/postgres"
	paymentsApp "github.com/VladHrytsaiuk/ecommerce-core/internal/payments/application"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/config"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/encryption"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/observability"
	eventsPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/platform/postgres/events"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/security/token"
	reportsOrderAnalytics "github.com/VladHrytsaiuk/ecommerce-core/internal/reports/adapter/orderanalytics"
	reportsApp "github.com/VladHrytsaiuk/ecommerce-core/internal/reports/application"
	reportsProjectors "github.com/VladHrytsaiuk/ecommerce-core/internal/reports/application/projectors"
	reportsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/reports/domain"
	reportsPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/reports/repository/postgres"
	returnsFinance "github.com/VladHrytsaiuk/ecommerce-core/internal/returns/adapter/finance"
	returnsInventory "github.com/VladHrytsaiuk/ecommerce-core/internal/returns/adapter/inventory"
	returnsOrderSnapshot "github.com/VladHrytsaiuk/ecommerce-core/internal/returns/adapter/ordersnapshot"
	returnsApp "github.com/VladHrytsaiuk/ecommerce-core/internal/returns/application"
	returnsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/returns/domain"
	returnsPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/returns/repository/postgres"
	searchCatalog "github.com/VladHrytsaiuk/ecommerce-core/internal/search/adapter/catalog"
	searchMeili "github.com/VladHrytsaiuk/ecommerce-core/internal/search/adapter/meilisearch"
	searchApp "github.com/VladHrytsaiuk/ecommerce-core/internal/search/application"
	searchDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/search/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/ratelimit"
	supportIdentity "github.com/VladHrytsaiuk/ecommerce-core/internal/support/adapter/identity"
	supportApp "github.com/VladHrytsaiuk/ecommerce-core/internal/support/application"
	supportPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/support/repository/postgres"
	videoCloudflare "github.com/VladHrytsaiuk/ecommerce-core/internal/video/adapter/cloudflare"
	videoApp "github.com/VladHrytsaiuk/ecommerce-core/internal/video/application"
	videoPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/video/repository/postgres"
)

// This file holds the assembly of each optional module.
//
// They live apart from Bootstrap because each is genuinely self-contained: a
// module reads configuration and the database handle, returns the services and
// workers it owns, and never reaches into another module's internals. Keeping
// them inline made the Composition Root a single long function in which the
// order of unrelated modules looked significant when it was not.
//
// What remains in Bootstrap is the part where order does matter: the shared
// commerce core that several modules depend on, and the final assembly.

// reportsRuntime is the analytics projection: a read model, its rebuilder and
// the worker that keeps it current.
type reportsRuntime struct {
	Queries   reportsDomain.QueryService
	Rebuilder *reportsApp.ReportsRebuilder
	Worker    *eventsApp.OutboxWorker
}

func buildReports(cfg *config.Config, db *gorm.DB) (reportsRuntime, error) {
	repository := reportsPostgres.NewRepository(db)
	queries, err := reportsApp.NewQueryService(repository, cfg.ReportsTimezone)
	if err != nil {
		return reportsRuntime{}, fmt.Errorf("configure reports queries: %w", err)
	}
	snapshots := reportsOrderAnalytics.NewProvider(db)
	rebuilder, err := reportsApp.NewReportsRebuilder(repository, repository, snapshots, cfg.ReportsTimezone)
	if err != nil {
		return reportsRuntime{}, fmt.Errorf("configure reports rebuilder: %w", err)
	}
	sales, err := reportsProjectors.NewDailySalesProjector(repository, repository, snapshots, cfg.ReportsTimezone)
	if err != nil {
		return reportsRuntime{}, fmt.Errorf("configure reports daily sales projector: %w", err)
	}
	refunds, err := reportsProjectors.NewRefundProjector(repository, repository, snapshots, cfg.ReportsTimezone)
	if err != nil {
		return reportsRuntime{}, fmt.Errorf("configure reports refund projector: %w", err)
	}
	cartFunnel, err := reportsProjectors.NewFunnelProjector(repository, repository, eventsDomain.TopicCartCreated, cfg.ReportsTimezone)
	if err != nil {
		return reportsRuntime{}, fmt.Errorf("configure reports cart funnel projector: %w", err)
	}
	checkoutFunnel, err := reportsProjectors.NewFunnelProjector(repository, repository, eventsDomain.TopicCheckoutStarted, cfg.ReportsTimezone)
	if err != nil {
		return reportsRuntime{}, fmt.Errorf("configure reports checkout funnel projector: %w", err)
	}
	return reportsRuntime{
		Queries:   queries,
		Rebuilder: rebuilder.WithLogger(logger.Log),
		Worker: eventsApp.NewOutboxWorker(eventsPostgres.NewDeliveryStore(db), eventsDomain.ConsumerReportsProjection, time.Minute, logger.Log,
			sales, refunds, cartFunnel, checkoutFunnel).WithTracer(observability.NewOutboxTracer()).WithMetrics(observability.NewOutboxMetrics()),
	}, nil
}

// searchRuntime is the external read projection. Index is returned separately
// so the caller registers it for release: it is a network client opened during
// composition, and a later failure must still close it.
type searchRuntime struct {
	Service          searchDomain.SearchService
	Worker           *eventsApp.OutboxWorker
	ProductPublisher eventsDomain.TransactionalEventPublisher
	Index            io.Closer
}

func buildSearch(cfg *config.Config, storeConfig StoreConfig, db *gorm.DB, products *catalogApp.ProductService) (searchRuntime, error) {
	// The index is contacted at startup, so a bounded deadline keeps an
	// unreachable Meilisearch from hanging the boot indefinitely.
	startupCtx, cancel := context.WithTimeout(context.Background(), searchMeili.DefaultTaskTimeout)
	index, err := searchMeili.New(startupCtx, searchMeili.Config{
		URL: cfg.SearchURL, MasterKey: cfg.SearchMasterKey,
		IndexUID: cfg.SearchIndexPrefix + "_" + storeConfig.Code + "_products",
	})
	cancel()
	if err != nil {
		return searchRuntime{}, fmt.Errorf("configure search index: %w", err)
	}
	handler, err := searchApp.NewProductChangedHandler(searchCatalog.NewSnapshotProvider(products), index)
	if err != nil {
		return searchRuntime{Index: index}, fmt.Errorf("configure search indexer: %w", err)
	}
	service, err := searchApp.NewSearchService(index)
	if err != nil {
		return searchRuntime{Index: index}, fmt.Errorf("configure search service: %w", err)
	}
	return searchRuntime{
		Service:          service,
		Worker:           eventsApp.NewOutboxWorker(eventsPostgres.NewDeliveryStore(db), eventsDomain.ConsumerSearchIndexer, time.Minute, logger.Log, handler).WithTracer(observability.NewOutboxTracer()).WithMetrics(observability.NewOutboxMetrics()),
		ProductPublisher: eventsPostgres.NewPublisher(eventsDomain.ConsumerSearchIndexer),
		Index:            index,
	}, nil
}

// mediaRuntime owns image storage. CatalogReader is returned rather than
// applied here so the caller can see that Catalog gains a media projection.
type mediaRuntime struct {
	Uploads       *mediaApp.UploadService
	CatalogReader *mediaApp.CatalogReader
	Worker        *eventsApp.OutboxWorker
	OrphanCleanup *mediaApp.OrphanCleanupWorker
}

func buildMedia(cfg *config.Config, db *gorm.DB) (mediaRuntime, error) {
	store, bucket, err := newMediaObjectStore(cfg)
	if err != nil {
		return mediaRuntime{}, fmt.Errorf("configure media object store: %w", err)
	}
	repository := mediaPostgres.NewRepository(db)
	uploads, err := mediaApp.NewUploadService(repository, store, adminPostgres.NewTransactionManager(db), eventsPostgres.NewPublisher(eventsDomain.ConsumerMediaProcessor), cfg.MediaProvider, bucket)
	if err != nil {
		return mediaRuntime{}, fmt.Errorf("configure media uploads: %w", err)
	}
	processor, err := mediaApp.NewAssetUploadedHandler(repository, mediaApp.NewPureGoProcessor(store))
	if err != nil {
		return mediaRuntime{}, fmt.Errorf("configure media processor: %w", err)
	}
	cleanup, err := mediaApp.NewOrphanCleanupWorker(repository, store, logger.Log)
	if err != nil {
		return mediaRuntime{}, fmt.Errorf("configure media orphan cleanup: %w", err)
	}
	return mediaRuntime{
		Uploads:       uploads,
		CatalogReader: mediaApp.NewCatalogReader(repository, store),
		Worker:        eventsApp.NewOutboxWorker(eventsPostgres.NewDeliveryStore(db), eventsDomain.ConsumerMediaProcessor, time.Minute, logger.Log, processor).WithTracer(observability.NewOutboxTracer()).WithMetrics(observability.NewOutboxMetrics()),
		OrphanCleanup: cleanup,
	}, nil
}

type videoRuntime struct {
	Uploads         *videoApp.DirectUploadService
	PlacementFacade *videoApp.PlacementAdminFacade
	Webhooks        *videoApp.WebhookService
	OrphanCleanup   *videoApp.OrphanCleanupWorker
	Storefront      *videoApp.StorefrontService
}

func buildVideo(cfg *config.Config, db *gorm.DB, authorizer adminDomain.Authorizer) (videoRuntime, error) {
	provider, err := videoCloudflare.New(videoCloudflare.Config{
		AccountID: cfg.CloudflareStreamAccountID, APIToken: cfg.CloudflareStreamAPIToken,
		WebhookSecret:  cfg.CloudflareStreamWebhookSecret,
		AllowedOrigins: cfg.CloudflareStreamAllowedOrigins,
	})
	if err != nil {
		return videoRuntime{}, fmt.Errorf("configure Cloudflare Stream video provider: %w", err)
	}
	repository := videoPostgres.NewRepository(db)
	signer, err := videoCloudflare.NewPlaybackSigner(videoCloudflare.SigningConfig{
		CustomerCode:  cfg.CloudflareStreamCustomerCode,
		KeyID:         cfg.CloudflareStreamSigningKeyID,
		PrivateKeyPEM: cfg.CloudflareStreamSigningKeyPEM,
	})
	if err != nil {
		return videoRuntime{}, fmt.Errorf("configure Cloudflare Stream playback signing: %w", err)
	}
	storefront, err := videoApp.NewStorefrontService(repository, signer, cfg.VideoPlaybackTTL)
	if err != nil {
		return videoRuntime{}, fmt.Errorf("configure video storefront playback: %w", err)
	}
	uploads, err := videoApp.NewDirectUploadService(repository, provider, cfg.VideoProvider)
	if err != nil {
		return videoRuntime{}, fmt.Errorf("configure video direct uploads: %w", err)
	}
	webhooks, err := videoApp.NewWebhookService(repository, provider, adminPostgres.NewTransactionManager(db), eventsPostgres.NewPublisher())
	if err != nil {
		return videoRuntime{}, fmt.Errorf("configure video webhooks: %w", err)
	}
	cleanup, err := videoApp.NewOrphanCleanupWorker(repository, provider, logger.Log)
	if err != nil {
		return videoRuntime{}, fmt.Errorf("configure video orphan cleanup: %w", err)
	}
	// Product video administration publishes to the Admin audit consumer,
	// which is why the video module requires admin to be enabled.
	placements, err := videoApp.NewPlacementAdminFacade(authorizer, repository, adminPostgres.NewTransactionManager(db), eventsPostgres.NewPublisher(eventsDomain.ConsumerAdminAudit))
	if err != nil {
		return videoRuntime{}, fmt.Errorf("configure video placement administration: %w", err)
	}
	return videoRuntime{
		Uploads: uploads, PlacementFacade: placements, Webhooks: webhooks,
		OrphanCleanup: cleanup, Storefront: storefront,
	}, nil
}

type returnsRuntime struct {
	Service *returnsApp.ReturnService
	Worker  *eventsApp.OutboxWorker
}

func buildReturns(storeConfig StoreConfig, db *gorm.DB, inventory *inventoryApp.Service, gateways *paymentsApp.Registry, notifications *notificationsPostgres.Repository) (returnsRuntime, error) {
	policy, err := returnsDomain.NewWindowEligibilityPolicy(storeConfig.ReturnWindowDays)
	if err != nil {
		return returnsRuntime{}, fmt.Errorf("configure returns policy: %w", err)
	}
	repository := returnsPostgres.NewRepository(db)
	snapshots := returnsOrderSnapshot.NewProvider(db)
	restock, err := returnsInventory.NewRestockPort(db, inventory, storeConfig.DefaultWarehouseID)
	if err != nil {
		return returnsRuntime{}, fmt.Errorf("configure returns restock: %w", err)
	}
	refunds, err := returnsFinance.NewRefundPort(db, gateways)
	if err != nil {
		return returnsRuntime{}, fmt.Errorf("configure returns refund: %w", err)
	}
	// The status topic is routed only where something can act on it. Without
	// the notifications module there is no mail to send, and a delivery for a
	// topic its consumer cannot handle is failed by the worker rather than
	// silently acknowledged — so an unrouted publisher is the correct shape,
	// and the event is still recorded for audit either way.
	statusPublisher := eventsPostgres.NewPublisher()
	handlers := []eventsApp.Consumer{}
	if notifications != nil {
		statusPublisher = eventsPostgres.NewPublisher(returnsDomain.ConsumerSettlement)
		notifier, notifierErr := returnsApp.NewStatusChangedNotifier(snapshots, adminPostgres.NewTransactionManager(db),
			notifications)
		if notifierErr != nil {
			return returnsRuntime{}, fmt.Errorf("configure returns status notifier: %w", notifierErr)
		}
		handlers = append(handlers, notifier)
	}
	service, err := returnsApp.NewReturnService(repository, snapshots, policy, adminPostgres.NewTransactionManager(db), statusPublisher, eventsPostgres.NewPublisher(returnsDomain.ConsumerSettlement))
	if err != nil {
		return returnsRuntime{}, fmt.Errorf("configure returns service: %w", err)
	}
	settlement, err := returnsApp.NewSettlementHandler(repository, snapshots, restock, refunds)
	if err != nil {
		return returnsRuntime{}, fmt.Errorf("configure returns settlement handler: %w", err)
	}
	confirmation, err := returnsApp.NewRefundConfirmedHandler(service)
	if err != nil {
		return returnsRuntime{}, fmt.Errorf("configure returns refund confirmation handler: %w", err)
	}
	// One worker for the module's own consumer. It carries settlement, refund
	// confirmation and — where notifications are enabled — the customer status
	// mail, because they are the same module's reactions to its own events.
	handlers = append(handlers, settlement, confirmation)
	return returnsRuntime{
		Service: service,
		Worker: eventsApp.NewOutboxWorker(eventsPostgres.NewDeliveryStore(db), returnsDomain.ConsumerSettlement, time.Minute, logger.Log,
			handlers...).WithTracer(observability.NewOutboxTracer()).WithMetrics(observability.NewOutboxMetrics()),
	}, nil
}

// notificationsRuntime supplies both the durable sender and the outbox
// consumer that enqueues into it.
type notificationsRuntime struct {
	Worker   *notificationsApp.DurableWorker
	Handlers []eventsApp.Consumer
}

func buildNotifications(cfg *config.Config, storeConfig StoreConfig, repository *notificationsPostgres.Repository) (notificationsRuntime, error) {
	cipher, err := encryption.NewAESGCM(cfg.NotificationEncryptionKey)
	if err != nil {
		return notificationsRuntime{}, fmt.Errorf("configure notifications encryption: %w", err)
	}
	sender, err := newNotificationEmailSender(cfg)
	if err != nil {
		return notificationsRuntime{}, fmt.Errorf("configure notifications email sender: %w", err)
	}
	// notification_templates ships empty and nothing seeded it, so every
	// lookup failed, every job retried ten times and died, and a store could
	// take orders without sending a single confirmation. The defaults go in
	// for the store's fallback locale — the one FindTemplate falls back to —
	// and never replace a template that is already there.
	if err := repository.SynchronizeTemplates(context.Background(), storeConfig.DefaultLocale, notificationsDomain.DefaultTemplates); err != nil {
		return notificationsRuntime{}, fmt.Errorf("install default notification templates: %w", err)
	}
	renderer := notificationsApp.NewTemplateRenderer(repository, storeConfig.DefaultLocale)
	return notificationsRuntime{
		Worker:   notificationsApp.NewDurableWorker(repository, renderer, sender).WithLogger(logger.Log),
		Handlers: []eventsApp.Consumer{notificationsApp.NewOrderPaidEventHandler(repository, cipher, renderer, sender)},
	}, nil
}

// abandonedCartRuntime also supplies checkout's contact capture, because the
// campaign is what gives a guest email a purpose worth collecting.
type abandonedCartRuntime struct {
	Worker         *abandonedApp.Worker
	OutboxWorker   *eventsApp.OutboxWorker
	ContactCapture checkoutDomain.ContactCaptureService
}

func buildAbandonedCart(cfg *config.Config, db *gorm.DB, consent *consentApp.Service, notifications *notificationsPostgres.Repository, cartRepositoryFor func() *cartApp.Service) (abandonedCartRuntime, error) {
	quietHours, err := abandonedApp.ParseQuietHours(cfg.AbandonedCartQuietHours)
	if err != nil {
		return abandonedCartRuntime{}, fmt.Errorf("configure abandoned-cart quiet hours: %w", err)
	}
	campaigns := abandonedPostgres.NewRepository(db)
	carts := abandonedReaders.NewCartReader(db)
	contacts := abandonedReaders.NewContactProvider(db)
	producer, err := abandonedApp.NewCampaignProducer(campaigns, carts, contacts, cfg.AbandonedCartDelays[0])
	if err != nil {
		return abandonedCartRuntime{}, fmt.Errorf("configure abandoned-cart producer: %w", err)
	}
	policy := abandonedApp.Policy{
		Delays:                  append([]time.Duration(nil), cfg.AbandonedCartDelays[:cfg.AbandonedCartMaxReminders]...),
		RequireMarketingConsent: cfg.AbandonedCartRequireMarketingConsent,
		QuietHours:              quietHours,
	}
	// Marketing mail must carry a way out. The link is minted from Consent's
	// signed capability, so this module cannot send without one — which is the
	// right coupling: a recovery campaign a guest cannot leave should not go at
	// all.
	linker, err := consentUnsubscribe.NewLinker(consent, cfg.FrontendURL)
	if err != nil {
		return abandonedCartRuntime{}, fmt.Errorf("configure abandoned-cart unsubscribe links: %w", err)
	}
	return abandonedCartRuntime{
		Worker: abandonedApp.NewWorker(campaigns, carts, abandonedReaders.NewConsentReader(db), notifications, adminPostgres.NewTransactionManager(db), policy, linker).WithLogger(logger.Log),
		OutboxWorker: eventsApp.NewOutboxWorker(eventsPostgres.NewDeliveryStore(db), abandonedApp.ConsumerCampaignProducer, time.Minute, logger.Log,
			producer, abandonedApp.NewTopicConsumer(eventsDomain.TopicCheckoutEmailCaptured, producer)).WithTracer(observability.NewOutboxTracer()).WithMetrics(observability.NewOutboxMetrics()),
		ContactCapture: checkoutApp.NewContactCaptureService(
			checkoutPostgres.NewContactRepository(db), cartRepositoryFor(),
			checkoutConsent.NewWriter(consent), adminPostgres.NewTransactionManager(db),
			eventsPostgres.NewPublisher(abandonedApp.ConsumerCampaignProducer)),
	}, nil
}

// identityRuntime is authentication plus the two optional profile surfaces.
// A profile service stays nil when its module is off, and the routes that
// consume it are then not registered.
type identityRuntime struct {
	Auth            *identityService.AuthService
	Profiles        identityDomain.ProfileService
	CustomerProfile identityDomain.CustomerProfileService
	// AttemptCleanup is not optional and not module-gated: every deployment
	// signs people in, and oauth_authorization_attempts had no sweep at all.
	AttemptCleanup *identityApplication.OAuthAttemptCleanup
	// RefreshTokenCleanup removes refresh tokens once their sign-in has ended.
	RefreshTokenCleanup *identityApplication.RefreshTokenCleanup
	// CodeCleanup runs whatever is enabled, so codes issued before a method
	// or the notifications module was switched off are still removed.
	CodeCleanup *identityApplication.CodeCleanup
}

// notifications is nil where the notifications module is off. There are no
// email codes then: registration sends no verification code, there is no
// password reset, and email_code is refused, here as in StoreConfig.
func buildIdentity(cfg *config.Config, storeConfig StoreConfig, db *gorm.DB, tokenMaker token.Maker, modules ModuleSet, observer identityDomain.UserLoginObserver, notifications *notificationsPostgres.Repository) (identityRuntime, error) {
	providers := make([]identityDomain.OAuthProvider, 0, 1)
	if storeConfig.AuthMethods.Has(AuthMethodGoogle) {
		google, err := googleAuthAdapter.New(googleAuthAdapter.Config{
			ClientID:            storeConfig.GoogleOAuth.ClientID,
			ClientSecret:        storeConfig.GoogleOAuth.ClientSecret,
			AllowedRedirectURIs: []string{storeConfig.GoogleOAuth.RedirectURI},
		})
		if err != nil {
			return identityRuntime{}, err
		}
		providers = append(providers, google)
	}
	auth := identityService.NewAuthService(
		identityPostgres.NewUserRepository(db),
		identityPostgres.NewOAuthIdentityRepository(db),
		identityPostgres.NewOAuthAttemptStore(db),
		identityPostgres.NewAuthTransaction(db),
		identityService.NewOAuthProviderRegistry(providers...),
		tokenMaker,
		cfg.AccessTokenDuration,
		storeConfig.OAuthAttemptTTL,
	)
	auth.WithRefreshTokens(identityPostgres.NewRefreshTokenStore(db), cfg.RefreshTokenDuration)
	auth.WithPasswordSignIn(storeConfig.AuthMethods.Has(AuthMethodPassword))
	senders := make([]identityDomain.CodeSender, 0, 2)
	if notifications != nil {
		// Codes are sent by email wherever mail is: they verify addresses after
		// registration and reset passwords, and sign in where email_code is on.
		mailer, err := identityNotifications.NewCodeMailer(notifications)
		if err != nil {
			return identityRuntime{}, fmt.Errorf("configure email codes: %w", err)
		}
		senders = append(senders, mailer)
	} else if storeConfig.AuthMethods.Has(AuthMethodEmailCode) {
		return identityRuntime{}, fmt.Errorf("configure email sign-in codes: the notifications module is required")
	}
	if storeConfig.AuthMethods.Has(AuthMethodPhoneCode) {
		texter, err := newSMSCodeTexter(storeConfig.SMS, codeTTL(cfg))
		if err != nil {
			return identityRuntime{}, fmt.Errorf("configure phone sign-in codes: %w", err)
		}
		senders = append(senders, texter)
	}
	if len(senders) > 0 {
		codes := identityService.CodeConfig{
			Store: identityPostgres.NewCodeStore(db), Accounts: identityPostgres.NewCodeAccounts(db),
			Secret: cfg.JWTSecret, TTL: codeTTL(cfg), Senders: senders,
		}
		if storeConfig.SMS != nil {
			codes.PhoneCountryCodes, codes.PhoneHourlyLimit = storeConfig.SMS.AllowedCountryCodes, storeConfig.SMS.HourlyLimit
		}
		if _, err := auth.WithCodes(codes); err != nil {
			return identityRuntime{}, fmt.Errorf("configure one-time codes: %w", err)
		}
	}
	auth.WithCodeSignIn(identityDomain.CodeChannelEmail, storeConfig.AuthMethods.Has(AuthMethodEmailCode))
	auth.WithCodeSignIn(identityDomain.CodeChannelPhone, storeConfig.AuthMethods.Has(AuthMethodPhoneCode))
	if observer != nil {
		auth.WithUserLoginObserver(observer)
	}
	attemptStore := identityPostgres.NewOAuthAttemptStore(db)
	attemptCleanup, err := identityApplication.NewOAuthAttemptCleanup(attemptStore)
	if err != nil {
		return identityRuntime{}, fmt.Errorf("configure OAuth attempt cleanup: %w", err)
	}
	refreshCleanup, err := identityApplication.NewRefreshTokenCleanup(identityPostgres.NewRefreshTokenStore(db))
	if err != nil {
		return identityRuntime{}, fmt.Errorf("configure refresh token cleanup: %w", err)
	}
	codeCleanup, err := identityApplication.NewCodeCleanup(identityPostgres.NewCodeStore(db))
	if err != nil {
		return identityRuntime{}, fmt.Errorf("configure one-time code cleanup: %w", err)
	}
	runtime := identityRuntime{Auth: auth, AttemptCleanup: attemptCleanup.WithLogger(logger.Log), RefreshTokenCleanup: refreshCleanup.WithLogger(logger.Log), CodeCleanup: codeCleanup.WithLogger(logger.Log)}
	if modules.Has(ModuleCustomers) {
		runtime.CustomerProfile = identityApplication.NewCustomerProfileService(identityPostgres.NewCustomerProfileRepository(db))
	}
	if modules.Has(ModuleUserProfiles) && storeConfig.ProfilePolicy != nil {
		runtime.Profiles = identityService.NewProfileService(*storeConfig.ProfilePolicy, identityPostgres.NewProfileRepository(db))
	}
	return runtime, nil
}

// availabilityRuntime is back-in-stock notification. The publisher is attached
// to Inventory by the caller, because that is where the stock transition that
// triggers a notification actually happens.
type availabilityRuntime struct {
	Service *availabilityApp.Service
	Worker  *eventsApp.OutboxWorker
}

func buildAvailability(storeConfig StoreConfig, db *gorm.DB, notifications *notificationsPostgres.Repository) availabilityRuntime {
	repository := availabilityPostgres.NewRepository(db)
	handler := availabilityApp.NewHandler(repository, notifications, func(ctx context.Context, fn func(context.Context) error) error {
		return adminPostgres.NewTransactionManager(db).WithinTransaction(ctx, fn)
	})
	return availabilityRuntime{
		Service: availabilityApp.NewService(repository, availabilityIdentity.NewEmailReader(db)),
		Worker: eventsApp.NewOutboxWorker(eventsPostgres.NewDeliveryStore(db), availabilityDomain.ConsumerAvailabilityNotifications, time.Minute, logger.Log,
			handler).WithTracer(observability.NewOutboxTracer()).WithMetrics(observability.NewOutboxMetrics()),
	}
}

// buildSupport wires public ticket intake. Its spam protector takes the shared
// limiter rather than a private one, so the quota holds across replicas.
func buildSupport(storeConfig StoreConfig, db *gorm.DB, limiter ratelimit.Service, notifications *notificationsPostgres.Repository) *supportApp.Service {
	return supportApp.NewService(
		supportPostgres.NewRepository(db),
		supportApp.NewSpamProtector(limiter),
		supportIdentity.NewEmailReader(db),
	).WithAdminWorkflow(adminPostgres.NewTransactionManager(db), notifications)
}

// buildConsent wires GDPR consent and privacy requests.
//
// Neither an ErasureExecutor nor a DataExporter is supplied, so this deployment
// refuses both request types at intake and refuses to approve any that predate
// the refusal. That is deliberate: what a store must delete, what it must
// retain, what belongs in an export and how it reaches the customer follow from
// its jurisdiction and its own commitments, not from this core. A deployment
// that has made those decisions attaches its implementations here with
// .WithErasure(...) and .WithExport(...), and those requests start being
// accepted.
//
// Refusing is the honest position. An export used to be accepted whatever the
// deployment could do: stored, approved, moved to in_progress, and left there
// while a statutory deadline ran against a request nothing would ever answer.
func buildConsent(cfg *config.Config, db *gorm.DB) (*consentApp.Service, error) {
	service := consentApp.NewService(
		consentPostgres.NewRepository(db),
		consentOrders.NewActivityReader(db),
	).WithAdminWorkflow(adminPostgres.NewTransactionManager(db), eventsPostgres.NewPublisher())
	// A guest has no session, so a signed link is the only thing that can
	// authorise them to stop receiving marketing. The service could already
	// withdraw consent by address and its comment claimed to "support a signed
	// unsubscribe endpoint", but nothing signed anything and no endpoint
	// existed: a guest sent an abandoned-cart email had no implemented way out.
	signer, err := consentDomain.NewUnsubscribeSigner(cfg.JWTSecret, unsubscribeTokenTTL)
	if err != nil {
		return nil, fmt.Errorf("configure unsubscribe signer: %w", err)
	}
	return service.WithUnsubscribeSigner(signer), nil
}

// unsubscribeTokenTTL outlives a mail run and a customer's inbox habits without
// leaving an unbounded capability in a message that can be forwarded.
const unsubscribeTokenTTL = 90 * 24 * time.Hour
