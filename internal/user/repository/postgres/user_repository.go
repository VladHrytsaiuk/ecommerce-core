package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/db"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/user/domain"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type userRepository struct {
	db     *gorm.DB
	logger logger.Logger
}

// NewUserRepository створює новий екземпляр GORM-репозиторію для користувачів.
// db: активне підключення до бази даних (GORM).
func NewUserRepository(db *gorm.DB, l logger.Logger) domain.UserRepository {
	return &userRepository{
		db:     db,
		logger: l,
	}
}

// getDB повертає транзакцію з контексту, якщо вона є, інакше звичайне підключення.
func (r *userRepository) getDB(ctx context.Context) *gorm.DB {
	return db.GetTx(ctx, r.db).WithContext(ctx)
}

// Create зберігає новий запис користувача у базі даних (таблиця "user").
// Перетворює помилки PostgreSQL (unique_violation) у доменні помилки (domain.ErrEmailAlreadyExists).
func (r *userRepository) Create(ctx context.Context, user *domain.User) error {
	err := r.getDB(ctx).Create(user).Error
	if err != nil {
		domainErr := r.handleUserUniqueErr(err)
		r.logger.Error("failed to create user",
			zap.Error(err),
			zap.String("email", user.Email),
		)
		return fmt.Errorf("failed to create user: %w", domainErr)
	}
	return nil
}

// FindAllByRole повертає користувачів із заданою роллю у порядку створення.
func (r *userRepository) FindAllByRole(ctx context.Context, roleID int) ([]domain.User, error) {
	var users []domain.User
	if err := r.getDB(ctx).Where("role_id = ?", roleID).Order("created_at ASC").Find(&users).Error; err != nil {
		return nil, fmt.Errorf("failed to find users by role: %w", err)
	}
	return users, nil
}

// FindByEmail шукає користувача за електронною поштою.
// Повертає domain.ErrUserNotFound, якщо запис не знайдено.
func (r *userRepository) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	var user domain.User
	err := r.getDB(ctx).Where("email = ?", email).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to find user by email: %w", err)
	}
	return &user, nil
}

// FindByPhone шукає користувача за номером телефону.
// Повертає domain.ErrUserNotFound, якщо запис не знайдено.
func (r *userRepository) FindByPhone(ctx context.Context, phone string) (*domain.User, error) {
	var user domain.User
	err := r.getDB(ctx).Where("phone = ?", phone).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to find user by phone: %w", err)
	}
	return &user, nil
}

// FindByID шукає користувача за його унікальним UUID.
func (r *userRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	var user domain.User
	err := r.getDB(ctx).Where("id = ?", id).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to find user by id: %w", err)
	}
	return &user, nil
}

// Update оновлює всі поля існуючого користувача.
// Використовує Save(), щоб гарантовано оновити всі поля, включаючи булеві (GORM Updates ігнорує false).
func (r *userRepository) Update(ctx context.Context, user *domain.User) error {
	err := r.getDB(ctx).Save(user).Error
	if err != nil {
		domainErr := r.handleUserUniqueErr(err)
		r.logger.Error("failed to update user",
			zap.Error(err),
			zap.String("user_id", user.ID.String()),
		)
		return fmt.Errorf("failed to update user: %w", domainErr)
	}
	return nil
}

// Delete виконує "м'яке видалення" (Soft Delete) користувача.
// Фізично запис залишається в БД, але поле DeletedAt заповнюється.
func (r *userRepository) Delete(ctx context.Context, id uuid.UUID) error {
	// GORM автоматично робить Soft Delete, оскільки у нас є DeletedAt.
	err := r.getDB(ctx).Delete(&domain.User{}, id).Error
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}
	return nil
}

// handleUserUniqueErr винесена спільна логіка перевірки унікальності.
func (r *userRepository) handleUserUniqueErr(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		// Тепер ми використовуємо точні назви констрейнтів із міграції 000003.
		if pgErr.ConstraintName == "user_email_key" {
			return domain.ErrEmailAlreadyExists
		}
		if pgErr.ConstraintName == "user_phone_key" {
			return domain.ErrPhoneAlreadyExists
		}
	}
	return err
}

// Atomic виконує кілька операцій в одній транзакції.
// Дозволяє об'єднати UserRepository та VerifyCodeRepository.
func (r *userRepository) Atomic(ctx context.Context, fn func(domain.UserRepository, domain.VerifyCodeRepository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Створюємо тимчасові репозиторії на основі об'єкта транзакції (tx)
		uRepo := NewUserRepository(tx, r.logger)
		cRepo := NewVerifyCodeRepository(tx, r.logger)
		return fn(uRepo, cRepo)
	})
}
