//go:build legacy && integration
// +build legacy,integration

package service

import (
	"context"
	"testing"
	"time"

	cartRepoPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/cart/repository/postgres"
	discountPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/discount/repository/postgres"
	discountSvc "github.com/VladHrytsaiuk/ecommerce-core/internal/discount/service"
	orderDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/order/domain"
	orderRepoPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/order/repository/postgres"
	paymentPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/payment/repository/postgres"
	paymentSvc "github.com/VladHrytsaiuk/ecommerce-core/internal/payment/service"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/config"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/db"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/email"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	userDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/user/domain"
	userRepoPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/user/repository/postgres"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// noopLogger імплементує logger.Logger інтерфейс для тестів.
type noopLogger struct{}

func (l *noopLogger) Debug(msg string, fields ...zap.Field) {}
func (l *noopLogger) Info(msg string, fields ...zap.Field)  {}
func (l *noopLogger) Warn(msg string, fields ...zap.Field)  {}
func (l *noopLogger) Error(msg string, fields ...zap.Field) {}
func (l *noopLogger) Fatal(msg string, fields ...zap.Field) {}

func (l *noopLogger) Debugf(template string, args ...interface{}) {}
func (l *noopLogger) Infof(template string, args ...interface{})  {}
func (l *noopLogger) Warnf(template string, args ...interface{})  {}
func (l *noopLogger) Errorf(template string, args ...interface{}) {}
func (l *noopLogger) Fatalf(template string, args ...interface{}) {}

func (l *noopLogger) Debugw(msg string, kvs ...interface{}) {}
func (l *noopLogger) Infow(msg string, kvs ...interface{})  {}
func (l *noopLogger) Warnw(msg string, kvs ...interface{})  {}
func (l *noopLogger) Errorw(msg string, kvs ...interface{}) {}
func (l *noopLogger) Fatalw(msg string, kvs ...interface{}) {}

func (l *noopLogger) Sync() error                            { return nil }
func (l *noopLogger) With(fields ...zap.Field) logger.Logger { return l }

func seedUser(t *testing.T, gormDB *gorm.DB, id uuid.UUID, email, name string, phone *string) {
	var phoneVal interface{} = nil
	if phone != nil {
		phoneVal = *phone
	}
	err := gormDB.Exec(`INSERT INTO "user" (id, email, password_hash, first_name, last_name, role_id, phone, is_phone_verified) 
		VALUES (?, ?, 'hashed', ?, 'Doe', 1, ?, true) ON CONFLICT DO NOTHING`,
		id, email, name, phoneVal).Error
	require.NoError(t, err)
}

func seedCatalog(t *testing.T, gormDB *gorm.DB, variationID, productID, brandID, catID uuid.UUID) {
	err := gormDB.Exec(`INSERT INTO brand (id, name) VALUES (?, 'Test Brand') ON CONFLICT DO NOTHING`, brandID).Error
	require.NoError(t, err)

	err = gormDB.Exec(`INSERT INTO category (id) VALUES (?) ON CONFLICT DO NOTHING`, catID).Error
	require.NoError(t, err)

	err = gormDB.Exec(`INSERT INTO product (id, slug, brand_id, category_id, is_active) 
		VALUES (?, 'test-product', ?, ?, true) ON CONFLICT DO NOTHING`, productID, brandID, catID).Error
	require.NoError(t, err)

	err = gormDB.Exec(`INSERT INTO product_translation (product_id, language_code, name, description) 
		VALUES (?, 'uk', 'Тестовий товар', 'Опис') ON CONFLICT DO NOTHING`, productID).Error
	require.NoError(t, err)

	err = gormDB.Exec(`INSERT INTO product_variation (id, product_id, sku, price, is_active) 
		VALUES (?, ?, 'TEST-SKU', 10000, true) ON CONFLICT DO NOTHING`, variationID, productID).Error
	require.NoError(t, err)
}

func seedVerifyCode(t *testing.T, gormDB *gorm.DB, userID *uuid.UUID, phone string) {
	err := gormDB.Exec(`INSERT INTO verify_code (id, user_id, target, type, code, is_used, expires_at, created_at) 
		VALUES (?, ?, ?, 'phone_verification', '112233', true, ?, ?) ON CONFLICT DO NOTHING`,
		uuid.New(), userID, phone, time.Now().Add(10*time.Minute), time.Now().Add(-1*time.Minute)).Error
	require.NoError(t, err)
}

func ptr[T any](v T) *T {
	return &v
}

// mockUserRepo обгортає UserRepository для симуляції поведінки при гостьовому чекауті
type mockUserRepo struct {
	userDomain.UserRepository
}

func (m *mockUserRepo) FindByPhone(ctx context.Context, phone string) (*userDomain.User, error) {
	// Симулюємо, що за телефоном користувача не знайдено, щоб перейти до пошуку по Email
	// (імітує стан перегонів / race condition)
	return nil, userDomain.ErrUserNotFound
}

func TestOrderService_Integration_DuplicatePhone(t *testing.T) {
	// 1. Запуск БД в Docker та застосування міграцій
	gormDB, cleanup := db.SetupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	l := &noopLogger{}

	// 2. Ініціалізація реальних репозиторіїв та сервісу
	orderRepo := orderRepoPostgres.NewOrderRepository(gormDB, l)
	cartRepo := cartRepoPostgres.NewCartRepository(gormDB, l)
	userRepo := userRepoPostgres.NewUserRepository(gormDB, l)
	verifyRepo := userRepoPostgres.NewVerifyCodeRepository(gormDB, l)
	emailProv := email.NewZapProvider("", l)
	promoRepo := discountPostgres.NewPromoRepository(gormDB, l)
	promoService := discountSvc.NewPromoService(promoRepo, l, gormDB)

	cfg := &config.Config{
		APIHost: "http://localhost:8080",
	}

	paymentRepo := paymentPostgres.NewPaymentRepository(gormDB, l)
	paymentService := paymentSvc.NewPaymentService(paymentRepo, orderRepo, emailProv, cfg, l)

	// Для тестів використовуємо mockUserRepo, щоб симулювати унікальний конфлікт при оновленні телефону
	wrappedUserRepo := &mockUserRepo{UserRepository: userRepo}
	orderSvc := NewOrderService(orderRepo, cartRepo, wrappedUserRepo, verifyRepo, promoRepo, promoService, paymentService, emailProv, cfg, l)

	// 3. Сидування каталогу товарів
	variationID := uuid.New()
	productID := uuid.New()
	brandID := uuid.New()
	catID := uuid.New()
	seedCatalog(t, gormDB, variationID, productID, brandID, catID)

	// =========================================================================
	// CASE 1: Авторизований чекаут з дублікатом телефону
	// =========================================================================
	t.Run("Authorized Checkout with Duplicate Phone", func(t *testing.T) {
		userAID := uuid.New()
		userBID := uuid.New()
		phoneNum := "+380991112233"

		// Створюємо Користувача А (у якого вже є цей номер)
		seedUser(t, gormDB, userAID, "usera@example.com", "UserA", &phoneNum)

		// Створюємо Користувача Б (телефон порожній)
		seedUser(t, gormDB, userBID, "userb@example.com", "UserB", nil)

		// Додаємо товар до кошика Користувача Б
		err := cartRepo.AddItem(ctx, &userBID, nil, variationID, 1)
		require.NoError(t, err)

		// Сидуємо підтверджений OTP-код для телефону
		seedVerifyCode(t, gormDB, &userBID, phoneNum)

		// Оформлюємо замовлення для Користувача Б
		input := orderDomain.CreateOrderInput{
			CustomerEmail:         "userb@example.com",
			CustomerFirstName:     "UserB",
			CustomerLastName:      "Doe",
			CustomerPhone:         phoneNum,
			DeliveryProvider:      "nova_poshta",
			DeliveryType:          "warehouse",
			DeliveryCityRef:       "city-ref",
			DeliveryCityName:      "Київ",
			DeliveryWarehouseRef:  "wh-ref",
			DeliveryWarehouseName: "Відділення 1",
		}

		result, err := orderSvc.CreateOrder(ctx, &userBID, nil, "uk", input)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotEmpty(t, result.OrderID)

		// Перевірки:
		// 1. Замовлення має бути прив'язане до User B
		order, err := orderRepo.FindByID(ctx, result.OrderID)
		require.NoError(t, err)
		require.NotNil(t, order.UserID)
		require.Equal(t, userBID, *order.UserID, "Замовлення має бути зв'язане з Користувачем Б")

		// 2. Профіль User B не повинен оновитися (телефон має лишатися порожнім)
		userB, err := userRepo.FindByID(ctx, userBID)
		require.NoError(t, err)
		require.True(t, userB.Phone == nil || *userB.Phone == "", "Телефон Користувача Б не повинен був оновитися через унікальний конфлікт")

		// 3. Профіль User A не постраждав
		userA, err := userRepo.FindByID(ctx, userAID)
		require.NoError(t, err)
		require.Equal(t, phoneNum, *userA.Phone)
	})

	// =========================================================================
	// CASE 2: Гостьовий чекаут з існуючим Email та дублікатом телефону
	// =========================================================================
	t.Run("Guest Checkout by Email with Duplicate Phone", func(t *testing.T) {
		userAID := uuid.New()
		userCID := uuid.New()
		phoneNum := "+380995556677"
		emailC := "userc@example.com"
		sessionID := "test-session-id"

		// Створюємо Користувача А (у якого вже є цей номер)
		seedUser(t, gormDB, userAID, "usera_case2@example.com", "UserA2", &phoneNum)

		// Створюємо Користувача C (телефон порожній, email = userc@example.com)
		seedUser(t, gormDB, userCID, emailC, "UserC", nil)

		// Додаємо товар до кошика за sessionID
		err := cartRepo.AddItem(ctx, nil, &sessionID, variationID, 1)
		require.NoError(t, err)

		// Сидуємо підтверджений OTP-код для телефону
		seedVerifyCode(t, gormDB, nil, phoneNum)

		// Оформлюємо замовлення як гість із email = userc@example.com
		input := orderDomain.CreateOrderInput{
			CustomerEmail:         emailC,
			CustomerFirstName:     "UserC",
			CustomerLastName:      "Doe",
			CustomerPhone:         phoneNum,
			DeliveryProvider:      "nova_poshta",
			DeliveryType:          "warehouse",
			DeliveryCityRef:       "city-ref",
			DeliveryCityName:      "Київ",
			DeliveryWarehouseRef:  "wh-ref",
			DeliveryWarehouseName: "Відділення 1",
		}

		result, err := orderSvc.CreateOrder(ctx, nil, &sessionID, "uk", input)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotEmpty(t, result.OrderID)

		// Перевірки:
		// 1. Замовлення має бути прив'язане до User C (знайдено за Email)
		order, err := orderRepo.FindByID(ctx, result.OrderID)
		require.NoError(t, err)
		require.NotNil(t, order.UserID)
		require.Equal(t, userCID, *order.UserID, "Замовлення має бути автоматично зв'язане з Користувачем C за Email")

		// 2. Профіль User C не повинен оновитися (телефон лишається порожнім)
		userC, err := userRepo.FindByID(ctx, userCID)
		require.NoError(t, err)
		require.True(t, userC.Phone == nil || *userC.Phone == "", "Телефон Користувача C не повинен був оновитися через унікальний конфлікт")
	})
}
