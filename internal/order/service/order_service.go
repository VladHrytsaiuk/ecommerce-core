//go:build legacy
// +build legacy

package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"time"

	cartDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/cart/domain"
	discountDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/discount/domain"
	orderDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/order/domain"
	paymentDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/payment/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/config"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/db"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/email"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/security/password"
	productDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/product/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/pagination"
	shipmentDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/shipment/domain"
	userDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/user/domain"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type orderService struct {
	orderRepo       orderDomain.OrderRepository
	cartRepo        cartDomain.CartRepository
	userRepo        userDomain.UserRepository
	verifyCodeRepo  userDomain.VerifyCodeRepository
	promoRepo       discountDomain.PromoRepository
	promoService    discountDomain.PromoService
	paymentService  paymentDomain.PaymentService
	shipmentService shipmentDomain.ShipmentService
	emailProvider   email.Provider
	cfg             *config.Config
	l               logger.Logger
}

// NewOrderService створює новий сервіс замовлень
func NewOrderService(
	orderRepo orderDomain.OrderRepository,
	cartRepo cartDomain.CartRepository,
	userRepo userDomain.UserRepository,
	verifyCodeRepo userDomain.VerifyCodeRepository,
	promoRepo discountDomain.PromoRepository,
	promoService discountDomain.PromoService,
	paymentService paymentDomain.PaymentService,
	shipmentService shipmentDomain.ShipmentService,
	emailProvider email.Provider,
	cfg *config.Config,
	l logger.Logger,
) orderDomain.OrderService {
	return &orderService{
		orderRepo:       orderRepo,
		cartRepo:        cartRepo,
		userRepo:        userRepo,
		verifyCodeRepo:  verifyCodeRepo,
		promoRepo:       promoRepo,
		promoService:    promoService,
		paymentService:  paymentService,
		shipmentService: shipmentService,
		emailProvider:   emailProvider,
		cfg:             cfg,
		l:               l,
	}
}

// CreateOrder — головна бізнес-логіка створення замовлення
func (s *orderService) CreateOrder(
	ctx context.Context,
	userID *uuid.UUID,
	sessionID *string,
	lang string,
	input orderDomain.CreateOrderInput,
) (*orderDomain.CreateOrderResult, error) {

	// 1. Валідація доставки
	if input.DeliveryProvider == "" || input.DeliveryCityRef == "" || input.DeliveryWarehouseRef == "" {
		return nil, orderDomain.ErrNoDeliveryData
	}

	// 2. Зчитування кошика
	var cart *cartDomain.Cart
	var variations []productDomain.ProductVariation
	var err error

	if userID != nil {
		cart, variations, err = s.cartRepo.GetByUserID(ctx, *userID, lang)
	} else if sessionID != nil && *sessionID != "" {
		cart, variations, err = s.cartRepo.GetBySessionID(ctx, *sessionID, lang)
	} else {
		return nil, orderDomain.ErrNoIdentifier
	}
	if err != nil {
		s.l.Errorw("failed to get cart for order", "error", err)
		return nil, err
	}

	if cart == nil || len(cart.Items) == 0 {
		return nil, orderDomain.ErrEmptyCart
	}

	// 3. Валідація: всі варіації активні
	variationMap := make(map[uuid.UUID]productDomain.ProductVariation, len(variations))
	for _, v := range variations {
		variationMap[v.ID] = v
	}

	for _, item := range cart.Items {
		v, ok := variationMap[item.VariationID]
		if !ok || !v.IsActive {
			s.l.Warnw("inactive variation in cart during checkout", "variation_id", item.VariationID)
			return nil, orderDomain.ErrInactiveItem
		}
	}

	// 4. Визначення отримувача та прив'язка до акаунта
	// Отримувач — ЗАВЖДИ береться з форми (можна замовити на іншу особу)
	orderFirstName := input.CustomerFirstName
	orderLastName := input.CustomerLastName
	orderEmail := input.CustomerEmail
	orderPhone := input.CustomerPhone

	// Якщо дані порожні (пройшли валідацію DTO) — повертаємо помилку домену для безпеки
	if orderEmail == "" || orderFirstName == "" || orderLastName == "" || orderPhone == "" {
		return nil, orderDomain.ErrNoGuestData
	}

	// --- ВЕРИФІКАЦІЯ ТЕЛЕФОНУ (КЛЮЧОВА ЛОГІКА - ПОЗА ТРАНЗАКЦІЄЮ) ---
	verificationRequired := true
	if userID != nil {
		user, err := s.userRepo.FindByID(ctx, *userID)
		if err != nil {
			s.l.Errorw("failed to find user for order verification", "error", err, "user_id", *userID)
			return nil, err
		}
		if user.Phone != nil && *user.Phone == orderPhone && user.IsPhoneVerified {
			verificationRequired = false
		}
	}

	if verificationRequired {
		verifyCode, err := s.verifyCodeRepo.FindLastByTarget(ctx, orderPhone, "phone_verification")
		if err != nil {
			s.l.Errorw("error fetching verification state for phone", "error", err, "phone", orderPhone)
			return nil, err
		}

		// Код мусить існувати, бути використаним, і це мало відбутися недавно (напр., за останні 30 хв)
		isValid := verifyCode != nil && verifyCode.IsUsed && time.Since(verifyCode.CreatedAt) < 30*time.Minute
		if !isValid {
			s.l.Warnw("order attempt without verified phone", "phone", orderPhone)
			return nil, userDomain.ErrPhoneNotVerified
		}
	}

	// 5. Калькуляція + Price Snapshotting
	var orderItems []orderDomain.OrderItem
	totalPrice := 0

	var promoResult *discountDomain.PromoCalculationResult
	var promoCodeStr *string
	var totalDiscount int

	if cart.PromoCodeID != nil {
		p, err := s.promoService.GetPromoByID(ctx, *cart.PromoCodeID)
		if err == nil && p != nil {
			err = s.promoService.ValidatePromoLimits(ctx, p.ID, userID, &orderEmail, &orderPhone)
			if err == nil {
				var promoItems []discountDomain.PromoItemInfo
				for _, item := range cart.Items {
					if v, ok := variationMap[item.VariationID]; ok {
						var brandID *uuid.UUID
						if v.Product.BrandID != uuid.Nil {
							brandID = &v.Product.BrandID
						}
						promoItems = append(promoItems, discountDomain.PromoItemInfo{
							VariationID: v.ID,
							ProductID:   v.ProductID,
							CategoryID:  v.Product.CategoryID,
							BrandID:     brandID,
							Price:       v.Price,
							Quantity:    item.Quantity,
						})
					}
				}
				promoResult, err = s.promoService.CalculateCartDiscount(ctx, p.ID, promoItems)
				if err == nil {
					promoCodeStr = &p.Code
					totalDiscount = promoResult.TotalDiscountAmount
				}
			} else {
				return nil, err
			}
		}
	}

	for _, cartItem := range cart.Items {
		v := variationMap[cartItem.VariationID]
		itemTotal := v.Price * cartItem.Quantity
		totalPrice += itemTotal

		discountAmt := 0
		if promoResult != nil {
			discountAmt = promoResult.ItemDiscounts[v.ID]
		}

		orderItems = append(orderItems, orderDomain.OrderItem{
			VariationID:     cartItem.VariationID,
			Price:           v.Price,
			Quantity:        cartItem.Quantity,
			TotalPrice:      itemTotal,
			DiscountAmount:  discountAmt,
			FinalTotalPrice: itemTotal - discountAmt,
		})
	}

	totalPrice -= totalDiscount

	// Перевірка мінімальної суми замовлення
	minOrderAmount, err := s.shipmentService.GetMinimumOrderAmount(ctx)
	if err == nil && minOrderAmount > 0 && totalPrice < minOrderAmount {
		s.l.Warnw("Order attempt below minimum amount", "total_price", totalPrice, "min_amount", minOrderAmount)
		return nil, orderDomain.ErrMinOrderAmountNotReached
	}

	delivery := &orderDomain.Delivery{
		Provider:      input.DeliveryProvider,
		DeliveryType:  input.DeliveryType,
		CityRef:       input.DeliveryCityRef,
		CityName:      input.DeliveryCityName,
		WarehouseRef:  input.DeliveryWarehouseRef,
		WarehouseName: input.DeliveryWarehouseName,
	}

	var result *orderDomain.CreateOrderResult

	// --- ТРАНЗАКЦІЯ: Прив'язка користувача + Створення замовлення ---
	err = s.orderRepo.WithTransaction(ctx, func(txCtx context.Context, txOrderRepo orderDomain.OrderRepository) error {
		var resolvedUserID *uuid.UUID
		isNewUser := false
		var setupToken *string

		// Дістаємо gorm.DB транзакції з контексту для створення вкладеної транзакції (SAVEPOINT)
		txDB := db.GetTx(txCtx, nil)

		if txDB != nil {
			// Вкладена транзакція (SAVEPOINT) для безпечних операцій з користувачем.
			// Якщо silentRegister або Update впаде (напр., unique_violation),
			// SAVEPOINT відкотиться, але ГОЛОВНА транзакція залишиться живою.
			nestedErr := txDB.Transaction(func(nestedTx *gorm.DB) error {
				nestedCtx := context.WithValue(txCtx, db.TxKey{}, nestedTx)

				if userID != nil {
					// Авторизований користувач — прив'язуємо замовлення до нього
					user, err := s.userRepo.FindByID(nestedCtx, *userID)
					if err != nil {
						return err
					}
					resolvedUserID = userID

					// Якщо у користувача НЕ БУЛО телефону у профілі — зберігаємо цей нововерифікований
					if user.Phone == nil || *user.Phone == "" {
						phoneUpdateErr := nestedTx.Transaction(func(phoneTx *gorm.DB) error {
							phoneCtx := context.WithValue(nestedCtx, db.TxKey{}, phoneTx)
							user.Phone = &orderPhone
							user.IsPhoneVerified = true
							return s.userRepo.Update(phoneCtx, user)
						})
						if phoneUpdateErr != nil {
							s.l.Warnw("failed to update user profile phone during checkout (likely duplicate phone)",
								"error", phoneUpdateErr,
								"user_id", user.ID,
							)
						}
					}
				} else {
					// Гостьовий чекаут
					// 1. Спочатку шукаємо за телефоном (верифікований OTP — найнадійніший ідентифікатор)
					userByPhone, err := s.userRepo.FindByPhone(nestedCtx, orderPhone)
					if err != nil && !errors.Is(err, userDomain.ErrUserNotFound) {
						return err
					}

					if userByPhone != nil {
						// Знайшли за телефоном — прив'язуємо замовлення до цього акаунта
						resolvedUserID = &userByPhone.ID
					} else {
						// 2. Якщо не знайшли за телефоном, шукаємо за email
						userByEmail, err := s.userRepo.FindByEmail(nestedCtx, orderEmail)
						if err != nil && !errors.Is(err, userDomain.ErrUserNotFound) {
							return err
						}

						if userByEmail != nil {
							// Знайшли за email — прив'язуємо замовлення до цього акаунта
							resolvedUserID = &userByEmail.ID

							// Якщо у акаунта немає телефону — оновлюємо (бо телефон верифікований і вільний)
							if userByEmail.Phone == nil || *userByEmail.Phone == "" {
								phoneUpdateErr := nestedTx.Transaction(func(phoneTx *gorm.DB) error {
									phoneCtx := context.WithValue(nestedCtx, db.TxKey{}, phoneTx)
									userByEmail.Phone = &orderPhone
									userByEmail.IsPhoneVerified = true
									return s.userRepo.Update(phoneCtx, userByEmail)
								})
								if phoneUpdateErr != nil {
									s.l.Warnw("failed to update guest-by-email profile phone during checkout",
										"error", phoneUpdateErr,
										"user_id", userByEmail.ID,
									)
								}
							}
						} else {
							// 3. Ні за телефоном, ні за email не знайшли — тиха реєстрація
							newUser, regErr := s.silentRegister(nestedCtx, orderFirstName, orderLastName, orderEmail, orderPhone)
							if regErr != nil {
								return regErr
							}
							resolvedUserID = &newUser.ID
							isNewUser = true
						}
					}
				}

				return nil
			})

			if nestedErr != nil {
				// Якщо SAVEPOINT відкотився (напр., через concurrent unique violation),
				// ГОЛОВНА транзакція залишається живою. Fallback до гостьового замовлення.
				s.l.Warnw("failed to process user details during checkout, falling back to guest order",
					"error", nestedErr,
					"phone", orderPhone,
					"email", orderEmail,
				)
				resolvedUserID = nil
				isNewUser = false
			} else if isNewUser && resolvedUserID != nil {
				// Якщо юзер новий і збережений успішно — генеруємо токен для пароля
				tokenStr := generateSecureToken(10)
				setupToken = &tokenStr

				verifyCode := &userDomain.VerifyCode{
					Type:      "password_setup",
					Target:    orderEmail,
					UserID:    resolvedUserID,
					Code:      tokenStr,
					ExpiresAt: time.Now().Add(24 * time.Hour),
				}
				if err := s.verifyCodeRepo.Create(txCtx, verifyCode); err != nil {
					s.l.Errorw("failed to create setup token for new user", "error", err, "user_id", resolvedUserID)
					setupToken = nil
				}
			}
		} else {
			s.l.Warn("txDB is nil, proceeding with order creation without user linking")
		}

		// Створення замовлення в основній транзакції
		order := &orderDomain.Order{
			UserID:         resolvedUserID,
			StatusID:       orderDomain.StatusPendingPayment,
			FirstName:      orderFirstName,
			LastName:       orderLastName,
			Email:          orderEmail,
			Phone:          orderPhone,
			TotalPrice:     totalPrice,
			PromoCode:      promoCodeStr,
			DiscountAmount: totalDiscount,
			AdminComment:   input.AdminComment,
			PayTypes:       input.PayTypes,
		}

		if err := txOrderRepo.Create(txCtx, order, orderItems, delivery); err != nil {
			return err
		}

		// Записуємо історію створення замовлення (null -> PendingPayment)
		if err := txOrderRepo.CreateStatusHistory(txCtx, &orderDomain.OrderStatusHistory{
			OrderID:    order.ID,
			ToStatusID: orderDomain.StatusPendingPayment,
			Source:     orderDomain.SourceSystem,
			Comment:    ptr("Замовлення створено"),
		}); err != nil {
			return err
		}

		if cart.PromoCodeID != nil && promoCodeStr != nil {
			if err := s.promoRepo.IncrementUsageAtomic(txCtx, *cart.PromoCodeID); err != nil {
				return err
			}
			if err := s.promoRepo.RecordUsage(txCtx, &discountDomain.PromoCodeUsage{
				PromoCodeID: *cart.PromoCodeID,
				OrderID:     order.ID,
				UserID:      resolvedUserID,
				Email:       &orderEmail,
				Phone:       &orderPhone,
			}); err != nil {
				return err
			}
		}

		paymentURL := s.paymentService.GeneratePaymentURL(order.ID, order.TotalPrice, order.OrderNumber, input.PayTypes)

		result = &orderDomain.CreateOrderResult{
			OrderID:     order.ID,
			OrderNumber: order.OrderNumber,
			TotalPrice:  totalPrice,
			Status:      "pending_payment",
			PaymentURL:  paymentURL,
			IsNewUser:   isNewUser,
			SetupToken:  setupToken,
		}

		s.l.Infow("Order created successfully",
			"order_id", order.ID,
			"order_number", order.OrderNumber,
			"total_price", totalPrice,
			"user_id", resolvedUserID,
			"is_new_user", isNewUser,
		)

		return nil
	})

	if err != nil {
		s.l.Errorw("failed to execute order creation transaction", "error", err)
		return nil, err
	}

	// 8. Очищення кошика (поза транзакцією — не критична операція)
	s.cleanupCart(ctx, userID, sessionID)

	return result, nil
}

// GetByID повертає замовлення за UUID
func (s *orderService) GetByID(ctx context.Context, orderID uuid.UUID) (*orderDomain.Order, error) {
	return s.orderRepo.FindByID(ctx, orderID)
}

// GetOrderStatusByID повертає лише статус замовлення
func (s *orderService) GetOrderStatusByID(ctx context.Context, orderID uuid.UUID) (int, error) {
	return s.orderRepo.GetOrderStatusByID(ctx, orderID)
}

// GetMyOrders повертає замовлення поточного користувача
func (s *orderService) GetMyOrders(ctx context.Context, userID uuid.UUID, pgn pagination.Params) ([]orderDomain.Order, pagination.Metadata, error) {
	orders, total, err := s.orderRepo.FindByUserID(ctx, userID, pgn)
	if err != nil {
		return nil, pagination.Metadata{}, err
	}

	meta := pagination.CalculateMetadata(total, pgn.Page, pgn.Limit)
	return orders, meta, nil
}

// CancelOrderByUser дозволяє користувачу скасувати своє замовлення
func (s *orderService) CancelOrderByUser(ctx context.Context, userID uuid.UUID, orderID uuid.UUID) error {
	order, err := s.orderRepo.FindByID(ctx, orderID)
	if err != nil {
		return err
	}

	if order.UserID == nil || *order.UserID != userID {
		return errors.New("unauthorized to cancel this order")
	}

	// Можна скасувати тільки якщо ще не створено ТТН
	if order.TTNNumber != "" {
		return orderDomain.ErrTTNAlreadyCreated
	}

	// Якщо замовлення вже скасовано або повернено
	if order.StatusID == orderDomain.StatusCancelled || order.StatusID == orderDomain.StatusRefunded {
		return orderDomain.ErrOrderAlreadyCancelled
	}

	isPaid := order.StatusID >= orderDomain.StatusPaid

	// Оновлюємо статус
	if err := s.orderRepo.UpdateStatus(ctx, order.ID, orderDomain.StatusCancelled); err != nil {
		s.l.Errorw("failed to update order status to cancelled", "error", err, "order_id", orderID)
		return err
	}

	// Записуємо історію скасування
	if err := s.orderRepo.CreateStatusHistory(ctx, &orderDomain.OrderStatusHistory{
		OrderID:      order.ID,
		FromStatusID: &order.StatusID,
		ToStatusID:   orderDomain.StatusCancelled,
		Source:       orderDomain.SourceUser,
	}); err != nil {
		s.l.Errorw("failed to create status history for cancellation", "error", err, "order_id", orderID)
		return err
	}

	s.l.Infow("Order cancelled by user", "order_id", orderID, "user_id", userID)

	// Якщо було оплачено, ініціюємо процес повернення коштів (заглушка)
	if isPaid {
		s.l.Infow("Order was paid, initiating refund process", "order_id", orderID)
		if err := s.paymentService.ProcessRefundStub(ctx, order.ID); err != nil {
			s.l.Errorw("failed to process refund stub for cancelled order", "error", err, "order_id", orderID)
			// Ми можемо не повертати помилку користувачу, бо замовлення вже скасовано,
			// але це потрібно моніторити
		}
	}

	return nil
}

// GeneratePaymentURL генерує посилання на оплату (без вибору конкретного методу)
func (s *orderService) GeneratePaymentURL(ctx context.Context, orderID uuid.UUID) (string, error) {
	order, err := s.orderRepo.FindByID(ctx, orderID)
	if err != nil {
		return "", err
	}
	return s.paymentService.GeneratePaymentURL(order.ID, order.TotalPrice, order.OrderNumber, order.PayTypes), nil
}

// ProcessPaymentTimeouts обробляє тайм-аути оплат (нагадування та скасування)
func (s *orderService) ProcessPaymentTimeouts(ctx context.Context) error {
	now := time.Now()

	// 1. Відправка нагадувань (замовлення старші за 20 хвилин)
	reminderThreshold := now.Add(-20 * time.Minute)
	ordersToRemind, err := s.orderRepo.GetOrdersForPaymentReminder(ctx, reminderThreshold)
	if err != nil {
		s.l.Errorw("failed to get orders for payment reminder", "error", err)
	} else {
		for _, order := range ordersToRemind {
			emailItems := make([]email.OrderItemEmail, len(order.Items))
			for i, item := range order.Items {
				emailItems[i] = email.OrderItemEmail{
					Name:     fmt.Sprintf("Variation %s", item.VariationID.String()[:8]), // В реальному житті тут має бути назва
					Quantity: item.Quantity,
					Price:    item.Price,
					Total:    item.TotalPrice,
				}
			}

			emailData := email.OrderEmailData{
				OrderNumber:  order.OrderNumber,
				CustomerName: order.FirstName + " " + order.LastName,
				Email:        order.Email,
				Phone:        order.Phone,
				TotalPrice:   order.TotalPrice,
				Items:        emailItems,
				PaymentURL:   s.paymentService.GeneratePaymentURL(order.ID, order.TotalPrice, order.OrderNumber, ""),
				CancelURL:    s.cfg.FrontendURL + "/orders/" + order.ID.String(),
			}

			if err := s.emailProvider.SendPaymentReminderEmail(order.Email, emailData); err != nil {
				s.l.Errorw("failed to send payment reminder email", "error", err, "order_id", order.ID)
				continue
			}

			if err := s.orderRepo.MarkPaymentReminderSent(ctx, order.ID); err != nil {
				s.l.Errorw("failed to mark payment reminder sent", "error", err, "order_id", order.ID)
			} else {
				s.l.Infow("Payment reminder sent successfully", "order_id", order.ID)
			}
		}
	}

	// 2. Скасування замовлень (старші за 30 хвилин)
	cancelThreshold := now.Add(-30 * time.Minute)
	ordersToCancel, err := s.orderRepo.GetOrdersForPaymentTimeout(ctx, cancelThreshold)
	if err != nil {
		s.l.Errorw("failed to get orders for payment timeout", "error", err)
	} else {
		for _, order := range ordersToCancel {
			if err := s.orderRepo.UpdateStatus(ctx, order.ID, orderDomain.StatusCancelled); err != nil {
				s.l.Errorw("failed to auto-cancel order due to payment timeout", "error", err, "order_id", order.ID)
			} else {
				// Записуємо історію
				fromStatus := orderDomain.StatusPendingPayment
				if histErr := s.orderRepo.CreateStatusHistory(ctx, &orderDomain.OrderStatusHistory{
					OrderID:      order.ID,
					FromStatusID: &fromStatus,
					ToStatusID:   orderDomain.StatusCancelled,
					Source:       orderDomain.SourceSystem,
					Comment:      ptr("Автоматичне скасування: таймаут оплати"),
				}); histErr != nil {
					s.l.Errorw("failed to create status history for auto-cancel", "error", histErr, "order_id", order.ID)
				}
				s.l.Infow("Order auto-cancelled due to payment timeout", "order_id", order.ID)
			}
		}
	}

	return nil
}

// silentRegister створює нового юзера без стандартних перевірок AuthService
func (s *orderService) silentRegister(ctx context.Context, firstName, lastName, guestEmail, phone string) (*userDomain.User, error) {
	// Генеруємо криптографічний пароль (≥32 символи)
	randomPass := generateSecurePassword(32)
	hashedPass, err := password.HashPassword(randomPass)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password for silent registration: %w", err)
	}

	user := &userDomain.User{
		ID:              uuid.New(),
		RoleID:          userDomain.RoleCustomer,
		FirstName:       firstName,
		LastName:        lastName,
		Email:           guestEmail,
		Phone:           &phone,
		PasswordHash:    hashedPass,
		AuthProvider:    "guest_checkout",
		IsEmailVerified: false,
		IsPhoneVerified: true, // Телефон вже перевірено вище перед викликом
		IsBlocked:       false,
	}

	if err := s.userRepo.Create(ctx, user); err != nil {
		return nil, fmt.Errorf("failed to create user via silent registration: %w", err)
	}

	s.l.Infow("Silent registration completed",
		"user_id", user.ID,
		"email", guestEmail,
	)

	return user, nil
}

// cleanupCart видаляє вміст кошика після створення замовлення
func (s *orderService) cleanupCart(ctx context.Context, userID *uuid.UUID, sessionID *string) {
	if userID != nil {
		if err := s.cartRepo.RemoveItem(ctx, userID, nil, uuid.Nil); err != nil {
			// Ігноруємо помилку — кошик може бути вже порожнім
			s.l.Warnw("cart cleanup: could not remove items by user_id (non-critical)", "error", err)
		}
	} else if sessionID != nil {
		if err := s.cartRepo.RemoveItem(ctx, nil, sessionID, uuid.Nil); err != nil {
			s.l.Warnw("cart cleanup: could not remove items by session_id (non-critical)", "error", err)
		}
	}
}

// generateSecurePassword генерує криптографічно стійкий пароль
func generateSecurePassword(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*"
	b := make([]byte, length)
	max := big.NewInt(int64(len(charset)))
	for i := range b {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			panic(fmt.Sprintf("crypto rand failure: %v", err))
		}
		b[i] = charset[n.Int64()]
	}
	return string(b)
}

// generateSecureToken генерує криптографічно стійкий токен (без спецсимволів) вказаної довжини
func generateSecureToken(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	max := big.NewInt(int64(len(charset)))
	for i := range b {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			panic(fmt.Sprintf("crypto rand failure: %v", err))
		}
		b[i] = charset[n.Int64()]
	}
	return string(b)
}

func ptr(s string) *string {
	return &s
}
