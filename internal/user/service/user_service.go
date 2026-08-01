package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/user/domain"
	"go.uber.org/zap"
)

type userService struct {
	userRepo    domain.UserRepository
	addressRepo domain.UserAddressRepository
	sessionRepo domain.SessionRepository
	logger      logger.Logger
}

// NewUserService створює новий екземпляр сервісу користувачів.
func NewUserService(
	userRepo domain.UserRepository,
	addressRepo domain.UserAddressRepository,
	sessionRepo domain.SessionRepository,
	l logger.Logger,
) domain.UserService {
	return &userService{
		userRepo:    userRepo,
		addressRepo: addressRepo,
		sessionRepo: sessionRepo,
		logger:      l,
	}
}

// GetMe повертає профіль користувача.
func (s *userService) GetMe(ctx context.Context, userID uuid.UUID) (*domain.User, error) {
	return s.userRepo.FindByID(ctx, userID)
}

// UpdateMe оновлює дані профілю. Дозволяє змінювати лише визначений набір полів (захист від Mass Assignment).
func (s *userService) UpdateMe(ctx context.Context, userID uuid.UUID, input *domain.UpdateMeInput) (*domain.User, error) {
	// 1. Отримуємо існуючого користувача
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	// 2. Копіюємо лише ті поля, які прийшли в запиті (не є nil)
	if input.FirstName != nil {
		user.FirstName = *input.FirstName
	}
	if input.LastName != nil {
		user.LastName = *input.LastName
	}
	if input.Phone != nil {
		if *input.Phone == "" {
			user.Phone = nil
		} else {
			user.Phone = input.Phone
		}
	}
	if input.WantsNewsletter != nil {
		user.WantsNewsletter = *input.WantsNewsletter
	}

	// 3. Зберігаємо оновлення
	err = s.userRepo.Update(ctx, user)
	if err != nil {
		return nil, err
	}

	return user, nil
}

// DeleteMe виконує Soft Delete акаунта та видаляє всі активні сесії.
func (s *userService) DeleteMe(ctx context.Context, userID uuid.UUID) error {
	// 1. Видаляємо всі сесії (примусовий розлогін)
	if err := s.sessionRepo.DeleteAllForUser(ctx, userID); err != nil {
		s.logger.Error("failed to delete sessions during account deletion", zap.Error(err), zap.String("user_id", userID.String()))
		// Продовжуємо далі, навіть якщо сесії не видалились (хоча це малоймовірно)
	}

	// 2. Виконуємо Soft Delete самого користувача
	return s.userRepo.Delete(ctx, userID)
}

// GetAddresses повертає список адрес користувача.
func (s *userService) GetAddresses(ctx context.Context, userID uuid.UUID) ([]domain.UserAddress, error) {
	return s.addressRepo.FindAllByUserID(ctx, userID)
}

// CreateAddress створює нову адресу в транзакції (якщо вона дефолтна).
func (s *userService) CreateAddress(ctx context.Context, address *domain.UserAddress) (*domain.UserAddress, error) {
	// Використовуємо Atomic для забезпечення транзакційності
	err := s.addressRepo.Atomic(ctx, func(txRepo domain.UserAddressRepository) error {
		// 1. Якщо адреса помічена як дефолтна, скидаємо інші тільки всередині транзакції
		if address.IsDefault {
			err := txRepo.UnsetDefaultAllForUser(ctx, address.UserID)
			if err != nil {
				return err
			}
		}

		// 2. Створюємо нову адресу
		return txRepo.Create(ctx, address)
	})

	if err != nil {
		return nil, err
	}

	return address, nil
}

// UpdateAddress оновлює існуючу адресу в транзакції.
func (s *userService) UpdateAddress(ctx context.Context, userID uuid.UUID, addressID uuid.UUID, address *domain.UserAddress) (*domain.UserAddress, error) {
	// 1. Попередньо перевіряємо приналежність адреси користувачу
	existing, err := s.addressRepo.FindByID(ctx, addressID)
	if err != nil {
		return nil, err
	}
	if existing.UserID != userID {
		return nil, fmt.Errorf("access denied to address")
	}

	address.ID = addressID
	address.UserID = userID

	// 2. Виконуємо оновлення в транзакції
	err = s.addressRepo.Atomic(ctx, func(txRepo domain.UserAddressRepository) error {
		// Якщо робимо адресу дефолтною, а вона раніше такою не була
		if address.IsDefault && !existing.IsDefault {
			err = txRepo.UnsetDefaultAllForUser(ctx, userID)
			if err != nil {
				return err
			}
		}
		return txRepo.Update(ctx, address)
	})

	if err != nil {
		return nil, err
	}

	return address, nil
}

// DeleteAddress видаляє адресу.
func (s *userService) DeleteAddress(ctx context.Context, userID uuid.UUID, addressID uuid.UUID) error {
	existing, err := s.addressRepo.FindByID(ctx, addressID)
	if err != nil {
		return err
	}
	if existing.UserID != userID {
		return fmt.Errorf("access denied to address")
	}

	return s.addressRepo.Delete(ctx, addressID)
}
