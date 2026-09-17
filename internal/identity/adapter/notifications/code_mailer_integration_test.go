//go:build integration

package notifications

import (
	"context"
	"encoding/json"
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
	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"
	containerPostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormLogger "gorm.io/gorm/logger"

	identity "github.com/VladHrytsaiuk/ecommerce-core/internal/identity/domain"
	identityPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/identity/repository/postgres"
	identityService "github.com/VladHrytsaiuk/ecommerce-core/internal/identity/service"
	notificationsApp "github.com/VladHrytsaiuk/ecommerce-core/internal/notifications/application"
	notifications "github.com/VladHrytsaiuk/ecommerce-core/internal/notifications/domain"
	notificationsPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/notifications/repository/postgres"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/encryption"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/security/token"
)

const (
	testEncryptionKey = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="
	testSecret        = "a-secure-secret-with-at-least-thirty-two-characters"
)

// What a customer does, against the real queue: ask for a code, read it in the
// email the worker would send, and sign in with it.
func TestACodeFromTheQueuedEmailSignsTheCustomerIn(t *testing.T) {
	db, auth, queue := newCodeSignIn(t)
	ctx := context.Background()

	if _, err := auth.RequestSignInCode(ctx, identity.RequestSignInCodeCommand{Channel: identity.CodeChannelEmail, Destination: "Buyer@Example.test"}); err != nil {
		t.Fatalf("RequestSignInCode() error = %v", err)
	}
	job, rendered := readQueuedEmail(t, queue)
	if job.Type != notifications.SignInCodeTemplate || job.Email != "buyer@example.test" {
		t.Fatalf("job = %+v, want a sign-in code email to the normalized address", job)
	}
	code := regexp.MustCompile(`\b\d{6}\b`).FindString(rendered.Text)
	if code == "" || regexp.MustCompile(`\d{6}`).MatchString(rendered.Subject) {
		t.Fatalf("rendered = %+v, want the code in the body and not in the subject", rendered)
	}
	if !regexp.MustCompile(`works for 10 minutes`).MatchString(rendered.Text) {
		t.Fatalf("text = %q, want the code's lifetime", rendered.Text)
	}

	session, err := auth.VerifySignInCode(ctx, identity.VerifySignInCodeCommand{Channel: identity.CodeChannelEmail, Destination: "buyer@example.test", Code: code})
	if err != nil || session.AccessToken == "" || session.RefreshToken == "" {
		t.Fatalf("VerifySignInCode() = (%+v, %v), want a session", session, err)
	}
	var account struct {
		EmailVerified bool
		Role          string
	}
	if err := db.Raw(`SELECT email_verified, role FROM users WHERE id = ?`, session.UserID).Scan(&account).Error; err != nil {
		t.Fatal(err)
	}
	if !account.EmailVerified || account.Role != string(identity.RoleCustomer) {
		t.Fatalf("account = %+v, want a verified customer", account)
	}

	// A second request is inside the cooldown: refused, and nothing queued.
	_, err = auth.RequestSignInCode(ctx, identity.RequestSignInCodeCommand{Channel: identity.CodeChannelEmail, Destination: "buyer@example.test"})
	var throttled *identity.CodeThrottledError
	if !errors.As(err, &throttled) {
		t.Fatalf("RequestSignInCode() inside the cooldown = %v, want throttled", err)
	}
	if again, err := queue.ClaimDue(ctx, time.Now().UTC().Add(time.Second)); err != nil || again != nil {
		t.Fatalf("ClaimDue() = (%+v, %v), want nothing queued by a throttled request", again, err)
	}
}

// The claim spans three stores — the code, the account, the refresh tokens —
// in one transaction, which only a real database can show holds together.
func TestACodeTakesOverAnAccountSomeoneRegisteredWithoutProvingTheAddress(t *testing.T) {
	db, auth, queue := newCodeSignIn(t)
	ctx := context.Background()
	squatter := uuid.New()
	if err := db.Exec(`INSERT INTO users (id, email, password_hash, role, status) VALUES (?, 'owner@example.test', 'squatter-hash', 'customer', 'active')`, squatter).Error; err != nil {
		t.Fatal(err)
	}
	if err := identityPostgres.NewRefreshTokenStore(db).Create(ctx, identity.NewRefreshToken{ID: uuid.New(), UserID: squatter, FamilyID: uuid.New(), Hash: make([]byte, 32), ExpiresAt: time.Now().Add(time.Hour), CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}

	if _, err := auth.RequestSignInCode(ctx, identity.RequestSignInCodeCommand{Channel: identity.CodeChannelEmail, Destination: "owner@example.test"}); err != nil {
		t.Fatal(err)
	}
	_, rendered := readQueuedEmail(t, queue)
	session, err := auth.VerifySignInCode(ctx, identity.VerifySignInCodeCommand{Channel: identity.CodeChannelEmail, Destination: "owner@example.test", Code: regexp.MustCompile(`\b\d{6}\b`).FindString(rendered.Text)})
	if err != nil || session.UserID != squatter {
		t.Fatalf("VerifySignInCode() = (%+v, %v), want the existing account", session, err)
	}
	var claimed int64
	if err := db.Raw(`SELECT COUNT(*) FROM users WHERE id = ? AND email_verified AND password_hash IS NULL`, squatter).Scan(&claimed).Error; err != nil {
		t.Fatal(err)
	}
	var open int64
	if err := db.Raw(`SELECT COUNT(*) FROM refresh_tokens WHERE user_id = ? AND revoked_at IS NULL`, squatter).Scan(&open).Error; err != nil {
		t.Fatal(err)
	}
	// The one open sign-in left is the one just issued.
	if claimed != 1 || open != 1 {
		t.Fatalf("claimed = %d, open sign-ins = %d; want the password gone and only the new sign-in open", claimed, open)
	}
}

// Registration sends the code in the transaction that creates the account; the
// code from that email confirms the address.
func TestTheCodeSentOnRegistrationConfirmsTheAddress(t *testing.T) {
	db, auth, queue := newCodeSignIn(t)
	ctx := context.Background()
	email := "New.Buyer@Example.test"

	session, err := auth.RegisterPassword(ctx, identity.RegisterPasswordCommand{Email: &email, Password: "correct-horse-battery"})
	if err != nil {
		t.Fatalf("RegisterPassword() error = %v", err)
	}
	job, rendered := readQueuedEmail(t, queue)
	if job.Type != notifications.EmailVerificationCodeTemplate || job.Email != "new.buyer@example.test" {
		t.Fatalf("job = %+v, want a verification email to the normalized address", job)
	}
	code := regexp.MustCompile(`\b\d{6}\b`).FindString(rendered.Text)

	if err := auth.ConfirmEmail(ctx, identity.ConfirmEmailCommand{UserID: session.UserID, Code: code}); err != nil {
		t.Fatalf("ConfirmEmail() error = %v", err)
	}
	var verified int64
	if err := db.Raw(`SELECT COUNT(*) FROM users WHERE id = ? AND email_verified AND password_hash IS NOT NULL`, session.UserID).Scan(&verified).Error; err != nil {
		t.Fatal(err)
	}
	if verified != 1 {
		t.Fatal("the address was not verified, or the password was lost")
	}
	if _, err := auth.RequestEmailVerification(ctx, session.UserID); !errors.Is(err, identity.ErrEmailAlreadyVerified) {
		t.Fatalf("RequestEmailVerification() after confirming = %v, want ErrEmailAlreadyVerified", err)
	}
}

// A reset read at the address replaces the password and ends the sign-ins of
// whoever had the old one.
func TestAResetCodeReplacesThePasswordAndEndsOtherSignIns(t *testing.T) {
	db, auth, queue := newCodeSignIn(t)
	ctx := context.Background()
	email := "reset@example.test"
	registered, err := auth.RegisterPassword(ctx, identity.RegisterPasswordCommand{Email: &email, Password: "old-password-123"})
	if err != nil {
		t.Fatal(err)
	}
	readQueuedEmail(t, queue) // the verification email

	// Registration was the address's last code; the reset waits out the
	// cooldown, as a person would.
	if err := db.Exec(`UPDATE one_time_codes SET created_at = created_at - interval '2 minutes', expires_at = expires_at - interval '2 minutes'`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := auth.RequestPasswordReset(ctx, identity.RequestPasswordResetCommand{Email: email}); err != nil {
		t.Fatalf("RequestPasswordReset() error = %v", err)
	}
	job, rendered := readQueuedEmail(t, queue)
	if job.Type != notifications.PasswordResetCodeTemplate {
		t.Fatalf("job = %+v, want a password reset email", job)
	}
	session, err := auth.ResetPassword(ctx, identity.ResetPasswordCommand{Email: email, Code: regexp.MustCompile(`\b\d{6}\b`).FindString(rendered.Text), Password: "new-password-456"})
	if err != nil || session.UserID != registered.UserID {
		t.Fatalf("ResetPassword() = (%+v, %v), want the account signed in", session, err)
	}

	if _, err := auth.RefreshSession(ctx, registered.RefreshToken); !errors.Is(err, identity.ErrInvalidRefreshToken) {
		t.Fatalf("RefreshSession(the sign-in before the reset) = %v, want it ended", err)
	}
	if _, err := auth.LoginPassword(ctx, identity.PasswordLoginCommand{Login: email, Password: "old-password-123"}); !errors.Is(err, identity.ErrInvalidCredentials) {
		t.Fatalf("LoginPassword(old password) = %v, want refused", err)
	}
	if _, err := auth.LoginPassword(ctx, identity.PasswordLoginCommand{Login: email, Password: "new-password-456"}); err != nil {
		t.Fatalf("LoginPassword(new password) = %v", err)
	}
}

func newCodeSignIn(t *testing.T) (*gorm.DB, *identityService.AuthService, *notificationsPostgres.Repository) {
	t.Helper()
	db := newDatabase(t)
	queue := newQueue(t, db)
	if err := queue.SynchronizeTemplates(context.Background(), "en", notifications.DefaultTemplates); err != nil {
		t.Fatal(err)
	}
	mailer, err := NewCodeMailer(queue)
	if err != nil {
		t.Fatal(err)
	}
	maker, err := token.NewJWTMaker(testSecret)
	if err != nil {
		t.Fatal(err)
	}
	auth := identityService.NewAuthService(identityPostgres.NewUserRepository(db), identityPostgres.NewOAuthIdentityRepository(db), identityPostgres.NewOAuthAttemptStore(db), identityPostgres.NewAuthTransaction(db), identityService.NewOAuthProviderRegistry(), maker, 15*time.Minute, 10*time.Minute).
		WithRefreshTokens(identityPostgres.NewRefreshTokenStore(db), 7*24*time.Hour)
	if _, err := auth.WithCodes(identityPostgres.NewCodeStore(db), identityPostgres.NewCodeAccounts(db), testSecret, 10*time.Minute, mailer); err != nil {
		t.Fatal(err)
	}
	auth.WithCodeSignIn(true)
	return db, auth, queue
}

// readQueuedEmail takes the next queued email and renders it as the worker
// would.
func readQueuedEmail(t *testing.T, queue *notificationsPostgres.Repository) (*notifications.DurableJob, notifications.EmailMessage) {
	t.Helper()
	ctx := context.Background()
	job, err := queue.ClaimDue(ctx, time.Now().UTC().Add(time.Second))
	if err != nil || job == nil {
		t.Fatalf("ClaimDue() = (%v, %v), want a queued email", job, err)
	}
	var payload any
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	rendered, err := notificationsApp.NewTemplateRenderer(queue, "en").Render(ctx, job.Type, job.Locale, payload)
	if err != nil {
		t.Fatalf("Render() error = %v; the default template and the payload disagree", err)
	}
	return job, rendered
}

func newQueue(t *testing.T, db *gorm.DB) *notificationsPostgres.Repository {
	t.Helper()
	cipher, err := encryption.NewAESGCM(testEncryptionKey)
	if err != nil {
		t.Fatal(err)
	}
	dedupe, err := notifications.NewDedupeKeyer(testEncryptionKey)
	if err != nil {
		t.Fatal(err)
	}
	return notificationsPostgres.NewRepository(db).WithDefaultLocale("en").WithEncryption(cipher, dedupe)
}

func newDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	testcontainers.SkipIfProviderIsNotHealthy(t)
	ctx := context.Background()
	container, err := containerPostgres.Run(ctx, "postgres:16-alpine",
		containerPostgres.WithDatabase("sign_in_codes"),
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
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(file), "..", "..", "..", "..")
	for _, step := range []struct{ dir, table string }{
		{filepath.Join(root, "migrations", "core"), "schema_migrations"},
		{filepath.Join(root, "migrations", "modules", "notifications"), "schema_migrations_module_notifications"},
	} {
		parsed, err := url.Parse(dsn)
		if err != nil {
			t.Fatal(err)
		}
		query := parsed.Query()
		query.Set("x-migrations-table", step.table)
		parsed.RawQuery = query.Encode()
		runner, err := migrate.New("file://"+step.dir, parsed.String())
		if err != nil {
			t.Fatal(err)
		}
		if err := runner.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			t.Fatal(err)
		}
		_, _ = runner.Close()
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: gormLogger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	return db
}
