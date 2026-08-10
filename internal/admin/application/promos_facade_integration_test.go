package application_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"
	postgresContainer "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	adminApplication "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/application"
	adminHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/delivery/http"
	adminPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/repository/postgres"
	eventsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
	eventsApplication "github.com/VladHrytsaiuk/ecommerce-core/internal/core/events/application"
	sharedMiddleware "github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware"
	eventsPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/platform/postgres/events"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/security/token"
	promosApplication "github.com/VladHrytsaiuk/ecommerce-core/internal/promos/application"
	promosPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/promos/repository/postgres"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/cache"
)

func TestPromosAdminHTTPPublishesAndPersistsAuditTrail(t *testing.T) {
	testcontainers.SkipIfProviderIsNotHealthy(t)
	ctx := context.Background()
	container, err := postgresContainer.Run(ctx, "postgres:16-alpine", postgresContainer.WithDatabase("admin_audit_test"), postgresContainer.WithUsername("admin"), postgresContainer.WithPassword("admin"), testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(30*time.Second)))
	if err != nil {
		t.Fatalf("start PostgreSQL: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(ctx) })
	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := applyAuditIntegrationSchema(db); err != nil {
		t.Fatal(err)
	}

	userID, roleID, permissionID := uuid.New(), uuid.New(), uuid.New()
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{"INSERT INTO users (id) VALUES (?)", []any{userID}},
		{"INSERT INTO admin_users (user_id, authorization_version) VALUES (?, 1)", []any{userID}},
		{"INSERT INTO roles (id, code, name) VALUES (?, 'promo-manager', 'Promo manager')", []any{roleID}},
		{"INSERT INTO permissions (id, code, resource, action) VALUES (?, 'promos:write', 'promos', 'write')", []any{permissionID}},
		{"INSERT INTO admin_user_roles (user_id, role_id) VALUES (?, ?)", []any{userID, roleID}},
		{"INSERT INTO role_permissions (role_id, permission_id) VALUES (?, ?)", []any{roleID, permissionID}},
	} {
		if err := db.Exec(statement.sql, statement.args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	authorizer, err := adminApplication.NewAuthorizer(adminPostgres.NewRepository(db), cache.NewNoOpService(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	facade, err := adminApplication.NewPromosAdminFacade(authorizer, promosApplication.NewAdminService(promosPostgres.NewRepository(db)), adminPostgres.NewTransactionManager(db), eventsPostgres.NewPublisher(eventsDomain.ConsumerAdminAudit))
	if err != nil {
		t.Fatal(err)
	}
	maker, err := token.NewJWTMaker("01234567890123456789012345678901")
	if err != nil {
		t.Fatal(err)
	}
	jwt, _, err := maker.CreateTokenForRole(userID, token.RoleCustomer, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	admin := router.Group("/api/admin")
	admin.Use(sharedMiddleware.AuthMiddleware(maker))
	adminHTTP.RegisterPromosRoutes(admin, authorizer, facade)
	request := httptest.NewRequest(http.MethodPost, "/api/admin/promos", bytes.NewBufferString(`{"code":"welcome10","discount_type":"percent","discount_value":1000}`))
	request.Header.Set("Authorization", "Bearer "+jwt)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("HTTP status = %d, body=%s", response.Code, response.Body.String())
	}
	var promotions, events, deliveries int64
	if err := db.Table("promocodes").Count(&promotions).Error; err != nil || promotions != 1 {
		t.Fatalf("promotions = %d, err=%v", promotions, err)
	}
	if err := db.Table("domain_events").Where("topic = ?", "admin.action.v1").Count(&events).Error; err != nil || events != 1 {
		t.Fatalf("outbox events = %d, err=%v", events, err)
	}
	if err := db.Table("event_deliveries").Where("consumer = ? AND status = 'pending'", eventsDomain.ConsumerAdminAudit).Count(&deliveries).Error; err != nil || deliveries != 1 {
		t.Fatalf("pending audit deliveries = %d, err=%v", deliveries, err)
	}
	worker := eventsApplication.NewOutboxWorker(eventsPostgres.NewDeliveryStore(db), eventsDomain.ConsumerAdminAudit, time.Minute, nil, adminApplication.NewAdminAuditEventHandler(adminPostgres.NewAuditRepository(db)))
	if err := worker.DispatchOnce(ctx); err != nil {
		t.Fatalf("dispatch audit event: %v", err)
	}
	var log struct {
		Action     string
		NewPayload string
	}
	if err := db.Raw("SELECT action, new_payload::text FROM audit_logs").Scan(&log).Error; err != nil {
		t.Fatal(err)
	}
	if log.Action != "promos.create" || !json.Valid([]byte(log.NewPayload)) || !bytes.Contains([]byte(log.NewPayload), []byte(`"code": "WELCOME10"`)) {
		t.Fatalf("audit log = %#v", log)
	}

	// A handler must sanitize again even if a malformed/manual producer somehow
	// bypasses the event constructor before delivery.
	unsafeEventID, unsafeResourceID := uuid.New(), uuid.New()
	unsafePayload := `{"version":1,"actor_user_id":"` + userID.String() + `","action":"promos.import","resource_type":"promo","resource_id":"` + unsafeResourceID.String() + `","old_payload":{"token":"must-not-persist"},"new_payload":{"nested":{"password":"must-not-persist"}},"occurred_at":"2026-01-01T00:00:00Z"}`
	if err := db.Exec(`INSERT INTO domain_events (id, topic, aggregate_type, aggregate_id, idempotency_key, payload, occurred_at) VALUES (?, 'admin.action.v1', 'promo', ?, ?, ?, CURRENT_TIMESTAMP)`, unsafeEventID, unsafeResourceID, uuid.New(), unsafePayload).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO event_deliveries (event_id, consumer, status, available_at) VALUES (?, ?, 'pending', CURRENT_TIMESTAMP)`, unsafeEventID, eventsDomain.ConsumerAdminAudit).Error; err != nil {
		t.Fatal(err)
	}
	var oldPayload, newPayload string
	for attempt := 0; attempt < 2; attempt++ {
		if err := worker.DispatchOnce(ctx); err != nil {
			t.Fatalf("dispatch defensive sanitization event: %v", err)
		}
		if err := db.Raw(`SELECT old_payload::text, new_payload::text FROM audit_logs WHERE event_id = ?`, unsafeEventID).Row().Scan(&oldPayload, &newPayload); err == nil {
			break
		}
	}
	if oldPayload == "" && newPayload == "" {
		t.Fatal("sanitized audit event was not persisted")
	}
	if bytes.Contains([]byte(oldPayload), []byte("must-not-persist")) || bytes.Contains([]byte(newPayload), []byte("must-not-persist")) {
		t.Fatalf("unsafe audit values persisted: old=%s new=%s", oldPayload, newPayload)
	}
}

func applyAuditIntegrationSchema(db *gorm.DB) error {
	if err := db.Exec("CREATE EXTENSION IF NOT EXISTS pgcrypto; CREATE TABLE users (id UUID PRIMARY KEY);").Error; err != nil {
		return err
	}
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return errors.New("locate audit migrations")
	}
	root := filepath.Join(filepath.Dir(file), "../../../")
	for _, migration := range []string{
		"migrations/modules/admin/000001_init_rbac.up.sql",
		"migrations/core/000005_add_event_outbox_and_order_contacts.up.sql",
		"migrations/modules/promos/000001_init_promos.up.sql",
		"migrations/modules/admin/000002_add_audit_logs.up.sql",
	} {
		raw, err := os.ReadFile(filepath.Join(root, migration))
		if err != nil {
			return err
		}
		// The outbox migration also owns order_contact_details, which is outside
		// this focused fixture and needs an orders table on a production schema.
		if migration == "migrations/core/000005_add_event_outbox_and_order_contacts.up.sql" {
			var eventSQL string
			eventSQL = string(raw[:bytes.Index(raw, []byte("-- Order contact"))])
			if err := db.Exec(eventSQL).Error; err != nil {
				return err
			}
			continue
		}
		if err := db.Exec(string(raw)).Error; err != nil {
			return err
		}
	}
	return nil
}
