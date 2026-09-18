//go:build integration

package sms

import (
	"context"
	"errors"
	"net/url"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/testcontainers/testcontainers-go"
	containerPostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormLogger "gorm.io/gorm/logger"

	mockSMS "github.com/VladHrytsaiuk/ecommerce-core/internal/adapters/notification/sms/mock"
	identity "github.com/VladHrytsaiuk/ecommerce-core/internal/identity/domain"
	identityPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/identity/repository/postgres"
	identityService "github.com/VladHrytsaiuk/ecommerce-core/internal/identity/service"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/security/token"
)

const testSecret = "a-secure-secret-with-at-least-thirty-two-characters"

// What a customer does with a phone: ask for a code, read it in the text, sign
// in — against the real code store and users table.
func TestATextedCodeSignsTheCustomerIn(t *testing.T) {
	db := newDatabase(t)
	ctx := context.Background()
	texts := mockSMS.New(nil)
	texter, err := NewCodeTexter(texts, "Code {code}, {minutes} min")
	if err != nil {
		t.Fatal(err)
	}
	maker, err := token.NewJWTMaker(testSecret)
	if err != nil {
		t.Fatal(err)
	}
	auth := identityService.NewAuthService(identityPostgres.NewUserRepository(db), identityPostgres.NewOAuthIdentityRepository(db), identityPostgres.NewOAuthAttemptStore(db), identityPostgres.NewAuthTransaction(db), identityService.NewOAuthProviderRegistry(), maker, 15*time.Minute, 10*time.Minute).
		WithRefreshTokens(identityPostgres.NewRefreshTokenStore(db), 7*24*time.Hour)
	auth.WithAccounts(identityPostgres.NewCodeAccounts(db))
	if _, err := auth.WithCodes(identityService.CodeConfig{
		Store: identityPostgres.NewCodeStore(db), Secret: testSecret, TTL: 5 * time.Minute,
		Senders: []identity.CodeSender{texter}, PhoneCountryCodes: []string{"380"}, PhoneHourlyLimit: 10,
	}); err != nil {
		t.Fatal(err)
	}
	auth.WithCodeSignIn(identity.CodeChannelPhone, true)

	if _, err := auth.RequestSignInCode(ctx, identity.RequestSignInCodeCommand{Channel: identity.CodeChannelPhone, Destination: "+380 50 123 45 67"}); err != nil {
		t.Fatalf("RequestSignInCode() error = %v", err)
	}
	sent := texts.Sent()
	if len(sent) != 1 || sent[0].Phone != "+380501234567" || !regexp.MustCompile(`^Code \d{6}, 5 min$`).MatchString(sent[0].Text) {
		t.Fatalf("texts = %+v, want one code to the normalized number", sent)
	}
	code := regexp.MustCompile(`\d{6}`).FindString(sent[0].Text)

	session, err := auth.VerifySignInCode(ctx, identity.VerifySignInCodeCommand{Channel: identity.CodeChannelPhone, Destination: "+380501234567", Code: code})
	if err != nil || session.RefreshToken == "" {
		t.Fatalf("VerifySignInCode() = (%+v, %v), want a session", session, err)
	}
	var account struct {
		Phone         string
		PhoneVerified bool
	}
	if err := db.Raw(`SELECT phone, phone_verified FROM users WHERE id = ?`, session.UserID).Scan(&account).Error; err != nil {
		t.Fatal(err)
	}
	if account.Phone != "+380501234567" || !account.PhoneVerified {
		t.Fatalf("account = %+v, want the number stored verified", account)
	}

	// The number is inside its cooldown: refused, and nothing texted.
	_, err = auth.RequestSignInCode(ctx, identity.RequestSignInCodeCommand{Channel: identity.CodeChannelPhone, Destination: "+380501234567"})
	var throttled *identity.CodeThrottledError
	if !errors.As(err, &throttled) || len(texts.Sent()) != 1 {
		t.Fatalf("RequestSignInCode() inside the cooldown = %v with %d texts, want throttled and none sent", err, len(texts.Sent()))
	}
}

func newDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	testcontainers.SkipIfProviderIsNotHealthy(t)
	ctx := context.Background()
	container, err := containerPostgres.Run(ctx, "postgres:16-alpine",
		containerPostgres.WithDatabase("phone_codes"),
		containerPostgres.WithUsername("identity"),
		containerPostgres.WithPassword("identity"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").
			WithOccurrence(2).WithStartupTimeout(60*time.Second)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })
	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	query.Set("x-migrations-table", "schema_migrations")
	parsed.RawQuery = query.Encode()
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(file), "..", "..", "..", "..")
	runner, err := migrate.New("file://"+filepath.Join(root, "migrations", "core"), parsed.String())
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatal(err)
	}
	_, _ = runner.Close()
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: gormLogger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	return db
}
