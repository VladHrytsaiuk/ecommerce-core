// Command admin — консольна утиліта для створення/підвищення користувача-адміністратора.
//
// Використання:
//
//	go run ./cmd/admin -email admin@aquawheel.store -password 'S3cret!' -first Alex -last Admin
//	go run ./cmd/admin -email existing@user.com -password 'NewPass1' -force   // підвищити наявного до адміна + змінити пароль
//
// Утиліта перевикористовує ті самі config/db/password-пакети, що й основний застосунок,
// тож хеш пароля (bcrypt) сумісний з логіном через API.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/config"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/db"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/security/password"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/user/domain"
	"gorm.io/gorm"
)

var emailRegex = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

const minPasswordLen = 8

func main() {
	logger.Init()
	defer func() { _ = logger.Log.Sync() }()

	var (
		email     = flag.String("email", "", "Email адміністратора (обов'язково)")
		pass      = flag.String("password", "", "Пароль адміністратора (мінімум 8 символів, обов'язково)")
		firstName = flag.String("first", "Admin", "Ім'я")
		lastName  = flag.String("last", "AquaWheel", "Прізвище")
		force     = flag.Bool("force", false, "Якщо користувач з таким email існує — підвищити до адміна та оновити пароль")
	)
	flag.Parse()

	if err := run(*email, *pass, *firstName, *lastName, *force); err != nil {
		logger.Log.Errorw("❌ Failed to create admin", "error", err)
		os.Exit(1)
	}
}

func run(email, pass, firstName, lastName string, force bool) error {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" || pass == "" {
		flag.Usage()
		return errors.New("прапорці -email та -password обов'язкові")
	}
	if !emailRegex.MatchString(email) {
		return fmt.Errorf("некоректний email: %q", email)
	}
	if len(pass) < minPasswordLen {
		return fmt.Errorf("пароль закороткий: мінімум %d символів", minPasswordLen)
	}

	cfg := config.Load()
	database, err := db.Connect(cfg.DBURL, db.DefaultPoolConfig())
	if err != nil {
		logger.Log.Fatal("Cannot connect to PostgreSQL")
	}
	sqlDB, err := database.DB()
	if err != nil {
		logger.Log.Fatal("Cannot obtain PostgreSQL pool")
	}
	defer func() { _ = sqlDB.Close() }()
	logger.Log.Info("✅ Database connection established")

	// 1. Ідемпотентно гарантуємо наявність ролей (сіду ролей у міграціях немає).
	if err := ensureRoles(database); err != nil {
		return fmt.Errorf("ensure roles: %w", err)
	}

	hash, err := password.HashPassword(pass)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	// 2. Перевіряємо, чи користувач уже існує.
	var existing domain.User
	err = database.Where("email = ?", email).First(&existing).Error
	switch {
	case err == nil:
		// Користувач існує.
		if !force {
			return fmt.Errorf("користувач %q вже існує; запустіть з -force, щоб підвищити до адміна та змінити пароль", email)
		}
		updates := map[string]any{
			"role_id":           domain.RoleAdmin,
			"password_hash":     hash,
			"is_email_verified": true,
			"is_blocked":        false,
			"auth_provider":     "local",
		}
		if err := database.Model(&domain.User{}).Where("id = ?", existing.ID).Updates(updates).Error; err != nil {
			return fmt.Errorf("update user: %w", err)
		}
		logger.Log.Infow("✅ Existing user promoted to admin", "email", email, "id", existing.ID)
		return nil

	case errors.Is(err, gorm.ErrRecordNotFound):
		// Створюємо нового адміна. ID/CreatedAt/UpdatedAt беруться з DB-дефолтів.
		user := domain.User{
			RoleID:          domain.RoleAdmin,
			FirstName:       firstName,
			LastName:        lastName,
			Email:           email,
			PasswordHash:    hash,
			IsEmailVerified: true,
			AuthProvider:    "local",
		}
		if err := database.Create(&user).Error; err != nil {
			return fmt.Errorf("create user: %w", err)
		}
		logger.Log.Infow("✅ Admin user created", "email", email, "id", user.ID)
		return nil

	default:
		return fmt.Errorf("lookup user: %w", err)
	}
}

// ensureRoles вставляє базові ролі (1=customer, 2=admin), якщо їх ще немає.
func ensureRoles(database *gorm.DB) error {
	return database.Exec(`
		INSERT INTO role (id, name, description) VALUES
			(1, 'customer', 'Default customer role'),
			(2, 'admin', 'Administrator role')
		ON CONFLICT (id) DO NOTHING
	`).Error
}
