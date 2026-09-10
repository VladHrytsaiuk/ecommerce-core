// Command admin bootstraps the first administrator account.
//
// Usage:
//
//	go run ./cmd/admin -email admin@example.com -password 'S3cret!'
//	go run ./cmd/admin -email existing@example.com -password 'NewPass1' -force
//
// It reuses the same config, database and password packages as the API, so the
// bcrypt hash it writes is accepted by the normal login endpoint. Granting
// access takes two steps that must agree: the Identity-owned users row carries
// the account and its role, while the Admin module's RBAC tables decide what
// that account may actually do.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/config"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/db"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/security/password"
)

var emailRegex = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

const (
	minPasswordLen = 8
	// bootstrapRole is seeded by the admin module's RBAC migrations and holds
	// every registered permission. Migrations create the role but deliberately
	// attach no user to it, which is the gap this command closes.
	bootstrapRole = "super_admin"
)

func main() {
	logger.Init()
	defer func() { _ = logger.Log.Sync() }()

	var (
		email = flag.String("email", "", "Administrator email (required)")
		pass  = flag.String("password", "", "Administrator password, at least 8 characters (required)")
		force = flag.Bool("force", false, "If the account already exists, promote it to administrator and reset its password")
	)
	flag.Parse()

	if err := run(*email, *pass, *force); err != nil {
		logger.Log.Errorw("failed to create administrator", "error", err)
		os.Exit(1)
	}
}

func run(email, plaintext string, force bool) error {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" || plaintext == "" {
		flag.Usage()
		return errors.New("-email and -password are required")
	}
	if !emailRegex.MatchString(email) {
		return fmt.Errorf("invalid email: %q", email)
	}
	// bcrypt silently truncates beyond 72 bytes, so a password the CLI accepts
	// but login would treat differently must be rejected here.
	if len(plaintext) < minPasswordLen || len(plaintext) > 72 {
		return fmt.Errorf("password must be between %d and 72 bytes", minPasswordLen)
	}

	cfg := config.Load()
	database, err := db.Connect(cfg.DBURL, db.DefaultPoolConfig())
	if err != nil {
		return fmt.Errorf("connect to PostgreSQL: %w", err)
	}
	sqlDB, err := database.DB()
	if err != nil {
		return fmt.Errorf("obtain PostgreSQL pool: %w", err)
	}
	defer func() { _ = sqlDB.Close() }()

	hash, err := password.HashPassword(plaintext)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	var userID uuid.UUID
	var created bool
	// One transaction so a half-granted administrator can never exist: an
	// account without its RBAC rows would pass authentication and then be
	// refused by every permission check.
	err = database.Transaction(func(tx *gorm.DB) error {
		userID, created, err = upsertUser(tx, email, hash, force)
		if err != nil {
			return err
		}
		return grantBootstrapRole(tx, userID)
	})
	if err != nil {
		return err
	}

	if created {
		logger.Log.Infow("administrator created", "email", email, "user_id", userID, "role", bootstrapRole)
	} else {
		logger.Log.Infow("existing account promoted to administrator", "email", email, "user_id", userID, "role", bootstrapRole)
	}
	return nil
}

// upsertUser creates the Identity account or, with -force, promotes an
// existing one. It reports whether the account was newly created.
func upsertUser(tx *gorm.DB, email, hash string, force bool) (uuid.UUID, bool, error) {
	var existing struct {
		ID uuid.UUID `gorm:"column:id"`
	}
	err := tx.Raw(`SELECT id FROM users WHERE email = ?`, email).Scan(&existing).Error
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("look up account: %w", err)
	}

	if existing.ID != uuid.Nil {
		if !force {
			return uuid.Nil, false, fmt.Errorf("account %q already exists; pass -force to promote it and reset its password", email)
		}
		result := tx.Exec(`
			UPDATE users
			   SET password_hash = ?, role = 'admin', status = 'active',
			       email_verified = TRUE, updated_at = CURRENT_TIMESTAMP
			 WHERE id = ?`, hash, existing.ID)
		if result.Error != nil {
			return uuid.Nil, false, fmt.Errorf("promote account: %w", result.Error)
		}
		return existing.ID, false, nil
	}

	var inserted struct {
		ID uuid.UUID `gorm:"column:id"`
	}
	err = tx.Raw(`
		INSERT INTO users (email, password_hash, role, status, email_verified)
		VALUES (?, ?, 'admin', 'active', TRUE)
		RETURNING id`, email, hash).Scan(&inserted).Error
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("create account: %w", err)
	}
	if inserted.ID == uuid.Nil {
		return uuid.Nil, false, errors.New("create account: database returned no identifier")
	}
	return inserted.ID, true, nil
}

// grantBootstrapRole registers the account with the Admin module and attaches
// the seeded bootstrap role. Re-running bumps authorization_version, which is
// what invalidates the cached permission set keyed on it.
func grantBootstrapRole(tx *gorm.DB, userID uuid.UUID) error {
	result := tx.Exec(`
		INSERT INTO admin_users (user_id, is_active)
		VALUES (?, TRUE)
		ON CONFLICT (user_id) DO UPDATE
		   SET is_active = TRUE,
		       authorization_version = admin_users.authorization_version + 1,
		       updated_at = CURRENT_TIMESTAMP`, userID)
	if result.Error != nil {
		return fmt.Errorf("register administrator: %w", result.Error)
	}

	result = tx.Exec(`
		INSERT INTO admin_user_roles (user_id, role_id)
		SELECT ?, role.id FROM roles AS role WHERE role.code = ?
		ON CONFLICT DO NOTHING`, userID, bootstrapRole)
	if result.Error != nil {
		return fmt.Errorf("attach %s role: %w", bootstrapRole, result.Error)
	}

	var granted int64
	if err := tx.Raw(`
		SELECT COUNT(*) FROM admin_user_roles AS admin_role
		  JOIN roles AS role ON role.id = admin_role.role_id
		 WHERE admin_role.user_id = ? AND role.code = ?`, userID, bootstrapRole).Scan(&granted).Error; err != nil {
		return fmt.Errorf("verify %s role: %w", bootstrapRole, err)
	}
	if granted == 0 {
		// The role is seeded by the admin module's migrations. Without it the
		// account would authenticate but hold no permission at all, so fail
		// loudly rather than leaving a useless administrator behind.
		return fmt.Errorf("role %q does not exist; run the admin module migrations first", bootstrapRole)
	}
	return nil
}
