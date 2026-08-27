//go:build integration

package orderworkflow

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"
	containerPostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	mockEmail "github.com/VladHrytsaiuk/ecommerce-core/internal/adapters/notification/email/mock"
	eventsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
	eventsApp "github.com/VladHrytsaiuk/ecommerce-core/internal/core/events/application"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
	workflowApp "github.com/VladHrytsaiuk/ecommerce-core/internal/core/orderworkflow/application"
	workflowDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/core/orderworkflow/domain"
	notificationsApp "github.com/VladHrytsaiuk/ecommerce-core/internal/notifications/application"
	notificationsPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/notifications/repository/postgres"
	ordersDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/orders/domain"
	paymentsApp "github.com/VladHrytsaiuk/ecommerce-core/internal/payments/application"
	paymentHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/payments/delivery/http"
	paymentsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/payments/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/encryption"
	eventsPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/platform/postgres/events"
	syncDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/sync/domain"
	syncPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/sync/repository/postgres"
)

func TestWorkflowPersistsOrderAndCommitsReservationExactlyOnce(t *testing.T) {
	testcontainers.SkipIfProviderIsNotHealthy(t)
	ctx := context.Background()
	container, err := containerPostgres.Run(ctx,
		"postgres:16-alpine",
		containerPostgres.WithDatabase("workflow_test"),
		containerPostgres.WithUsername("workflow"),
		containerPostgres.WithPassword("workflow"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(30*time.Second)),
	)
	if err != nil {
		t.Fatalf("start PostgreSQL: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	databaseURL, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("get PostgreSQL connection string: %v", err)
	}
	root := repositoryRoot(t)
	if err := applyMigrations(filepath.Join(root, "migrations", "core"), databaseURL, "schema_migrations"); err != nil {
		t.Fatalf("migrate core: %v", err)
	}
	if err := applyMigrations(filepath.Join(root, "migrations", "modules", "inventory"), databaseURL, "schema_migrations_module_inventory"); err != nil {
		t.Fatalf("migrate inventory: %v", err)
	}
	if err := applyMigrations(filepath.Join(root, "migrations", "modules", "sync"), databaseURL, "schema_migrations_module_sync"); err != nil {
		t.Fatalf("migrate sync: %v", err)
	}
	if err := applyMigrations(filepath.Join(root, "migrations", "modules", "notifications"), databaseURL, "schema_migrations_module_notifications"); err != nil {
		t.Fatalf("migrate notifications: %v", err)
	}

	db, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	seedOrderPaidTemplate(t, db)
	variantID, _, reservationID := seedReservation(t, db)
	price := mustMoney(1000, "EUR")
	service := workflowApp.NewService(NewRepository(db, true).WithEventPublisher(eventsPostgres.NewPublisher(eventsDomain.ConsumerNotifications)))
	order, err := service.CreatePendingCheckout(ctx, ordersDomain.Draft{
		Number: "ES-300", Subtotal: price, Tax: mustMoney(0, "EUR"), Total: price, PaymentProvider: "fake", DeliveryProvider: "novaposhta",
		Delivery: &ordersDomain.DeliveryDetails{RecipientName: "Iryna Customer", RecipientPhone: "+34123456789", CountryCode: "ES", City: "Madrid", LocalityID: "madrid", ServicePointID: "branch-1"},
		Contact:  &ordersDomain.ContactDetails{Email: "iryna@example.com", Locale: "es"},
		Items:    []ordersDomain.Item{{VariantID: &variantID, ProductName: "Cream", SKU: "CREAM-50", Quantity: 1, UnitPrice: price, Total: price, UnitWeightGrams: 275}},
	}, []uuid.UUID{reservationID}, workflowDomain.CheckoutAttemptRequest{Provider: "fake", IdempotencyKey: "checkout-300", Amount: price})
	if err != nil {
		t.Fatalf("CreatePending() error = %v", err)
	}
	confirmation := workflowDomain.PaymentConfirmation{PaymentAttempt: workflowDomain.PaymentAttempt{OrderID: order.ID, Provider: "fake", ProviderReference: "payment-300", Amount: price}, Status: "paid"}
	if err := service.RegisterPayment(ctx, confirmation.PaymentAttempt); err != nil {
		t.Fatalf("RegisterPayment() error = %v", err)
	}
	assertOrderAndReservation(t, db, order.ID, reservationID, "pending_payment", "active", 5, 1)

	if err := service.MarkPaid(ctx, confirmation); err != nil {
		t.Fatalf("MarkPaid() error = %v", err)
	}
	if err := service.MarkPaid(ctx, confirmation); err != nil {
		t.Fatalf("idempotent MarkPaid() error = %v", err)
	}
	assertOrderAndReservation(t, db, order.ID, reservationID, "paid", "committed", 4, 0)
	var jobs int
	var recipient string
	var unitWeightGrams int
	var paymentStatus, paymentReference string
	if err := db.Raw(`SELECT COUNT(*) FROM delivery_jobs WHERE order_id = ? AND provider = 'novaposhta' AND status = 'pending'`, order.ID).Scan(&jobs).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Raw(`SELECT recipient_name FROM order_delivery_details WHERE order_id = ?`, order.ID).Scan(&recipient).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Raw(`SELECT unit_weight_grams FROM order_items WHERE order_id = ?`, order.ID).Scan(&unitWeightGrams).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Raw(`SELECT status, provider_reference FROM payments WHERE order_id = ?`, order.ID).Row().Scan(&paymentStatus, &paymentReference); err != nil {
		t.Fatal(err)
	}
	if jobs != 1 || recipient != "Iryna Customer" || unitWeightGrams != 275 || paymentStatus != "paid" || paymentReference != "payment-300" {
		t.Fatalf("delivery snapshot/jobs/weight/payment = %q/%d/%d/%s/%s", recipient, jobs, unitWeightGrams, paymentStatus, paymentReference)
	}
	assertOrderCreatedOutbox(t, db, order.ID)
	assertSyncPersistence(t, ctx, db, order.ID)
	assertOrderPaidEventAndDispatch(t, ctx, db, order.ID)
}

func assertOrderPaidEventAndDispatch(t *testing.T, ctx context.Context, db *gorm.DB, orderID uuid.UUID) {
	t.Helper()
	var eventID uuid.UUID
	var topic, payload, status, consumer string
	if err := db.Raw(`SELECT event.id, event.topic, event.payload, delivery.consumer, delivery.status FROM domain_events event JOIN event_deliveries delivery ON delivery.event_id = event.id WHERE event.aggregate_id = ? AND event.topic = ?`, orderID, eventsDomain.TopicOrderPaid).Row().Scan(&eventID, &topic, &payload, &consumer, &status); err != nil {
		t.Fatal(err)
	}
	if topic != eventsDomain.TopicOrderPaid || consumer != eventsDomain.ConsumerNotifications || status != "pending" || !strings.Contains(payload, orderID.String()) {
		t.Fatalf("order paid event = %s/%s/%s/%s, want notifications pending event", eventID, topic, consumer, status)
	}
	sender := mockEmail.New(nil)
	cipher, err := encryption.NewAESGCM(testNotificationEncryptionKey(t))
	if err != nil {
		t.Fatalf("create notification cipher: %v", err)
	}
	repository := notificationsPostgres.NewRepository(db)
	handler := notificationsApp.NewOrderPaidEventHandler(repository, cipher, notificationsApp.NewTemplateRenderer(repository, "en"), sender)
	if err := eventsApp.NewOutboxWorker(eventsPostgres.NewDeliveryStore(db), eventsDomain.ConsumerNotifications, time.Minute, nil, handler).DispatchOnce(ctx); err != nil {
		t.Fatalf("dispatch order paid event: %v", err)
	}
	if err := db.Raw(`SELECT status FROM event_deliveries WHERE event_id = ? AND consumer = ?`, eventID, eventsDomain.ConsumerNotifications).Scan(&status).Error; err != nil {
		t.Fatal(err)
	}
	if status != "done" {
		t.Fatalf("event delivery status = %s, want done", status)
	}
	var ciphertext, jobStatus, attemptStatus string
	var recipient sql.NullString
	var jobs, attempts, attemptNumber int
	if err := db.Raw(`SELECT COUNT(*), MIN(recipient), MIN(payload_ciphertext), MIN(status) FROM notification_jobs WHERE order_id = ?`, orderID).Row().Scan(&jobs, &recipient, &ciphertext, &jobStatus); err != nil {
		t.Fatal(err)
	}
	if err := db.Raw(`SELECT COUNT(*), MIN(status), MIN(attempt_number) FROM notification_attempts`).Row().Scan(&attempts, &attemptStatus, &attemptNumber); err != nil {
		t.Fatal(err)
	}
	if jobs != 1 || attempts != 1 || attemptNumber != 1 || recipient.Valid || ciphertext == "" || strings.Contains(ciphertext, "iryna@example.com") || jobStatus != "sent" || attemptStatus != "success" || len(sender.Sent()) != 1 || sender.Sent()[0].Subject != "Receipt ES-300" {
		t.Fatalf("notification state jobs/attempts/attempt-number/plain-recipient/ciphertext/job/attempt/sent = %d/%d/%d/%t/%q/%s/%s/%d", jobs, attempts, attemptNumber, recipient.Valid, ciphertext, jobStatus, attemptStatus, len(sender.Sent()))
	}
}

func seedOrderPaidTemplate(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := db.Exec(`INSERT INTO notification_templates (template_key, channel, locale, version, subject_template, html_template, text_template) VALUES ('order_paid', 'email', 'en', 1, 'Receipt {{.OrderNumber}}', '<p>Your order {{.OrderNumber}} has been paid.</p>', 'Your order {{.OrderNumber}} has been paid.')`).Error; err != nil {
		t.Fatalf("seed notification template: %v", err)
	}
}

func testNotificationEncryptionKey(t *testing.T) string {
	t.Helper()
	return base64.StdEncoding.EncodeToString([]byte("12345678901234567890123456789012"))
}

func TestLatePaidWebhookWithoutPersistedPaymentCreatesOneAnomaly(t *testing.T) {
	testcontainers.SkipIfProviderIsNotHealthy(t)
	ctx := context.Background()
	container, err := containerPostgres.Run(ctx,
		"postgres:16-alpine",
		containerPostgres.WithDatabase("workflow_anomaly_test"),
		containerPostgres.WithUsername("workflow"),
		containerPostgres.WithPassword("workflow"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(30*time.Second)),
	)
	if err != nil {
		t.Fatalf("start PostgreSQL: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	databaseURL, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("get PostgreSQL connection string: %v", err)
	}
	if err := applyMigrations(filepath.Join(repositoryRoot(t), "migrations", "core"), databaseURL, "schema_migrations"); err != nil {
		t.Fatalf("migrate core: %v", err)
	}
	db, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}

	orderID := uuid.New()
	if err := db.Exec(`INSERT INTO orders (id, number, status, currency, subtotal_amount, tax_amount, shipping_amount, total_amount, payment_provider, expires_at) VALUES (?, 'ES-late-paid', 'cancelled', 'EUR', 1000, 0, 0, 1000, 'fake', ?)`, orderID, time.Now().UTC().Add(-time.Minute)).Error; err != nil {
		t.Fatal(err)
	}
	service := workflowApp.NewService(NewRepository(db, false))
	event := paymentsDomain.PaymentEvent{EventID: "late-paid-event", Provider: "fake", OrderID: orderID, ProviderReference: "captured-after-expiry", Status: "paid", Amount: mustMoney(1000, "EUR")}
	registry, err := paymentsApp.NewRegistry([]string{"fake"}, "fake", latePaidGateway{event: event})
	if err != nil {
		t.Fatalf("create gateway registry: %v", err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	paymentHTTP.RegisterWebhookRoutes(router.Group("/api"), paymentsApp.NewWebhookService(registry, &latePaidEventStore{}, service))
	for attempt := 0; attempt < 2; attempt++ {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/webhooks/payments/fake", nil))
		if recorder.Code != http.StatusNoContent {
			t.Fatalf("late paid webhook status = %d, want %d", recorder.Code, http.StatusNoContent)
		}
	}

	var anomalies, payments int
	if err := db.Raw(`SELECT COUNT(*) FROM payment_anomalies WHERE order_id = ? AND provider = 'fake' AND provider_reference = 'captured-after-expiry' AND reason = 'paid_after_cancelled'`, orderID).Scan(&anomalies).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Raw(`SELECT COUNT(*) FROM payments WHERE order_id = ?`, orderID).Scan(&payments).Error; err != nil {
		t.Fatal(err)
	}
	if anomalies != 1 || payments != 0 {
		t.Fatalf("anomaly/payment records = %d/%d, want 1/0", anomalies, payments)
	}
}

type latePaidGateway struct{ event paymentsDomain.PaymentEvent }

func (g latePaidGateway) Code() string { return "fake" }

func (latePaidGateway) CreateCheckout(context.Context, paymentsDomain.CheckoutPayment) (paymentsDomain.PaymentSession, error) {
	return paymentsDomain.PaymentSession{}, nil
}

func (g latePaidGateway) VerifyWebhook(context.Context, paymentsDomain.WebhookRequest) (paymentsDomain.PaymentEvent, error) {
	return g.event, nil
}

func (latePaidGateway) Refund(context.Context, paymentsDomain.RefundRequest) error { return nil }

type latePaidEventStore struct{}

func (*latePaidEventStore) Claim(context.Context, string, paymentsDomain.PaymentEvent) (bool, error) {
	return true, nil
}

func (*latePaidEventStore) MarkProcessed(context.Context, string, string) error { return nil }
func (*latePaidEventStore) Abandon(context.Context, string, string) error       { return nil }

func assertOrderCreatedOutbox(t *testing.T, db *gorm.DB, orderID uuid.UUID) {
	t.Helper()
	var topic, status, payload string
	if err := db.Raw(`SELECT topic, status, payload FROM sync_outbox WHERE aggregate_id = ?`, orderID).Row().Scan(&topic, &status, &payload); err != nil {
		t.Fatal(err)
	}
	if topic != "order.created" || status != "pending" || !strings.Contains(payload, `"order_id"`) || strings.Contains(payload, "Iryna Customer") {
		t.Fatalf("sync outbox = %q/%q/%s, want a PII-free pending order.created event", topic, status, payload)
	}
}

func assertSyncPersistence(t *testing.T, ctx context.Context, db *gorm.DB, orderID uuid.UUID) {
	t.Helper()
	outbox := syncPostgres.NewOutboxStore(db)
	event, err := outbox.Claim(ctx, time.Now().UTC(), time.Minute)
	if err != nil || event == nil || event.AggregateID != orderID || event.Topic != syncDomain.TopicOrderCreated {
		t.Fatalf("claim order event = (%+v, %v)", event, err)
	}
	if duplicate, err := outbox.Claim(ctx, time.Now().UTC(), time.Minute); err != nil || duplicate != nil {
		t.Fatalf("concurrent claim = (%+v, %v), want no second claim", duplicate, err)
	}
	if err := outbox.Complete(ctx, event.ID, time.Now().UTC()); err != nil {
		t.Fatalf("complete order event: %v", err)
	}

	crashedID := uuid.New()
	if err := db.Exec(`INSERT INTO sync_outbox (id, topic, aggregate_id, idempotency_key, payload, status, attempts, locked_at) VALUES (?, 'order.created', ?, ?, '{}', 'processing', 1, ?)`, crashedID, uuid.New(), uuid.New(), time.Now().UTC().Add(-2*time.Minute)).Error; err != nil {
		t.Fatal(err)
	}
	if recovered, err := outbox.Claim(ctx, time.Now().UTC(), time.Minute); err != nil || recovered == nil || recovered.ID != crashedID || recovered.Attempts != 2 {
		t.Fatalf("recover expired lease = (%+v, %v)", recovered, err)
	}

	states := syncPostgres.NewInboundStateStore(db)
	change := syncDomain.StockChange{Source: "1c", ExternalID: "sku-42", Version: "42", SourceUpdatedAt: time.Now().UTC(), PayloadHash: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", VariantID: uuid.New(), WarehouseID: uuid.New(), Quantity: 7}
	claimed, err := states.ClaimStockChange(ctx, change, time.Now().UTC(), time.Minute)
	if err != nil || !claimed {
		t.Fatalf("claim inbound change = (%t, %v)", claimed, err)
	}
	if err := states.MarkStockChangeApplied(ctx, change); err != nil {
		t.Fatalf("mark inbound change applied: %v", err)
	}
	if claimed, err := states.ClaimStockChange(ctx, change, time.Now().UTC(), time.Minute); err != nil || claimed {
		t.Fatalf("duplicate inbound change = (%t, %v), want false, nil", claimed, err)
	}
	conflict := change
	conflict.PayloadHash = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	if _, err := states.ClaimStockChange(ctx, conflict, time.Now().UTC(), time.Minute); !errors.Is(err, syncDomain.ErrInboundVersionConflict) {
		t.Fatalf("version conflict = %v", err)
	}
	stale := change
	stale.Version = "41"
	stale.SourceUpdatedAt = stale.SourceUpdatedAt.Add(-time.Second)
	if claimed, err := states.ClaimStockChange(ctx, stale, time.Now().UTC(), time.Minute); err != nil || claimed {
		t.Fatalf("stale inbound change = (%t, %v), want false, nil", claimed, err)
	}
}

func seedReservation(t *testing.T, db *gorm.DB) (uuid.UUID, uuid.UUID, uuid.UUID) {
	t.Helper()
	variantID, warehouseID, reservationID := uuid.New(), uuid.New(), uuid.New()
	productID := uuid.New()
	if err := db.Exec(`INSERT INTO locales (code, name, is_default) VALUES ('en', 'English', true)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO products (id, status) VALUES (?, 'active')`, productID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO product_variants (id, product_id, sku, status, price_amount, currency) VALUES (?, ?, 'CREAM-50', 'active', 1000, 'EUR')`, variantID, productID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO warehouses (id, code, name) VALUES (?, 'main', 'Main')`, warehouseID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO stock_items (id, variant_id, warehouse_id, quantity_on_hand, quantity_reserved) VALUES (?, ?, ?, 5, 1)`, uuid.New(), variantID, warehouseID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO inventory_reservations (id, idempotency_key, variant_id, warehouse_id, quantity, expires_at) VALUES (?, ?, ?, ?, 1, ?)`, reservationID, uuid.New(), variantID, warehouseID, time.Now().Add(time.Hour)).Error; err != nil {
		t.Fatal(err)
	}
	return variantID, warehouseID, reservationID
}

func assertOrderAndReservation(t *testing.T, db *gorm.DB, orderID, reservationID uuid.UUID, wantOrderStatus, wantReservationStatus string, wantOnHand, wantReserved int) {
	t.Helper()
	var orderStatus, reservationStatus string
	var reservationOrderID *uuid.UUID
	var onHand, reserved int
	if err := db.Raw(`SELECT status FROM orders WHERE id = ?`, orderID).Scan(&orderStatus).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Raw(`SELECT status, order_id FROM inventory_reservations WHERE id = ?`, reservationID).Row().Scan(&reservationStatus, &reservationOrderID); err != nil {
		t.Fatal(err)
	}
	if err := db.Raw(`SELECT quantity_on_hand, quantity_reserved FROM stock_items LIMIT 1`).Row().Scan(&onHand, &reserved); err != nil {
		t.Fatal(err)
	}
	if orderStatus != wantOrderStatus || reservationStatus != wantReservationStatus || reservationOrderID == nil || *reservationOrderID != orderID || onHand != wantOnHand || reserved != wantReserved {
		t.Fatalf("state = order:%s reservation:%s reservationOrder:%v stock:%d/%d", orderStatus, reservationStatus, reservationOrderID, onHand, reserved)
	}
}

func applyMigrations(dir, databaseURL, table string) error {
	m, err := migrate.New("file://"+dir, migrationURL(databaseURL, table))
	if err != nil {
		return err
	}
	defer func() { _, _ = m.Close() }()
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}
	return nil
}

func migrationURL(databaseURL, table string) string {
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		panic(err)
	}
	query := parsed.Query()
	query.Set("x-migrations-table", table)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("discover repository root")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
}

func mustMoney(amount int64, currency string) money.Money {
	value, err := money.NewMoney(amount, currency)
	if err != nil {
		panic(err)
	}
	return value
}
