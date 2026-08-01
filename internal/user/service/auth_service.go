package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/google/uuid"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/config"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/email"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/security/password"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/security/token"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/sms"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/user/domain"
	"go.uber.org/zap"
	"google.golang.org/api/idtoken"
)

type authService struct {
	userRepo       domain.UserRepository
	sessionRepo    domain.SessionRepository
	verifyCodeRepo domain.VerifyCodeRepository
	emailProvider  email.Provider
	smsSender      sms.Sender
	tokenMaker     token.Maker
	wsService      domain.WSNotifyService
	cfg            *config.Config
	logger         logger.Logger
}

// NewAuthService створює новий екземпляр сервісу автентифікації.
func NewAuthService(
	userRepo domain.UserRepository,
	sessionRepo domain.SessionRepository,
	verifyCodeRepo domain.VerifyCodeRepository,
	emailProvider email.Provider,
	smsSender sms.Sender,
	tokenMaker token.Maker,
	wsService domain.WSNotifyService,
	cfg *config.Config,
	l logger.Logger,
) domain.AuthService {
	return &authService{
		userRepo:       userRepo,
		sessionRepo:    sessionRepo,
		verifyCodeRepo: verifyCodeRepo,
		emailProvider:  emailProvider,
		smsSender:      smsSender,
		tokenMaker:     tokenMaker,
		wsService:      wsService,
		cfg:            cfg,
		logger:         l,
	}
}

// Register реєструє нового користувача.
func (s *authService) Register(ctx context.Context, user *domain.User) (*domain.AuthResponseData, error) {
	// 1. Форсування безпечних дефолтних значень
	user.RoleID = 1
	user.IsBlocked = false
	user.IsEmailVerified = false
	user.IsPhoneVerified = false
	user.AuthProvider = "local"
	if user.Phone != nil && *user.Phone == "" {
		user.Phone = nil
	}

	if len(user.PasswordHash) < 8 {
		return nil, fmt.Errorf("password must be at least 8 characters long")
	}

	// 2. Хешування пароля
	hashedPassword, err := password.HashPassword(user.PasswordHash)
	if err != nil {
		s.logger.Error("failed to hash password", zap.Error(err))
		return nil, domain.ErrInternal
	}
	user.PasswordHash = hashedPassword

	// 3. Створення користувача в БД
	err = s.userRepo.Create(ctx, user)
	if err != nil {
		return nil, err
	}

	// 4. Генерація та відправка коду верифікації
	err = s.sendVerification(ctx, user)
	if err != nil {
		s.logger.Error("failed to send initial verification email", zap.Error(err))
		// Ми не видаляємо юзера, просто логуємо помилку. Він зможе натиснути "Resend" пізніше.
	}

	// 5. ВАЖЛИВО: Ми більше не логінимо юзера автоматично після реєстрації.
	// Він має підтвердити пошту, щоб отримати токени.
	return &domain.AuthResponseData{
		User: *user, // Повертаємо дані юзера, але токени порожні
	}, nil
}

// CreateAdmin створює активний локальний обліковий запис адміністратора або Власника.
// Пароль задає чинний адміністратор, тому лист-підтвердження не надсилається.
func (s *authService) CreateAdmin(ctx context.Context, user *domain.User) (*domain.User, error) {
	if len(user.PasswordHash) < 8 {
		return nil, fmt.Errorf("password must be at least 8 characters long")
	}

	hashedPassword, err := password.HashPassword(user.PasswordHash)
	if err != nil {
		s.logger.Error("failed to hash admin password", zap.Error(err))
		return nil, domain.ErrInternal
	}

	if user.RoleID != domain.RoleAdmin && user.RoleID != domain.RoleOwner {
		return nil, domain.ErrForbiddenRole
	}
	user.PasswordHash = hashedPassword
	user.IsBlocked = false
	user.IsEmailVerified = true
	user.IsPhoneVerified = false
	user.AuthProvider = "local"
	user.Phone = nil

	if err := s.userRepo.Create(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

func (s *authService) ListAdmins(ctx context.Context) ([]domain.User, error) {
	admins, err := s.userRepo.FindAllByRole(ctx, domain.RoleAdmin)
	if err != nil {
		return nil, err
	}
	owners, err := s.userRepo.FindAllByRole(ctx, domain.RoleOwner)
	if err != nil {
		return nil, err
	}
	return append(owners, admins...), nil
}

// DeleteAdmin soft-deletes a regular administrator and invalidates all their sessions.
// Owners cannot be deleted through the admin panel, preventing the last owner lockout.
func (s *authService) DeleteAdmin(ctx context.Context, actorID, userID uuid.UUID) error {
	if actorID == userID {
		return domain.ErrForbiddenRole
	}
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	if user.RoleID != domain.RoleAdmin {
		return domain.ErrForbiddenRole
	}
	if err := s.sessionRepo.DeleteAllForUser(ctx, userID); err != nil {
		s.logger.Error("failed to clear admin sessions", zap.Error(err), zap.String("user_id", userID.String()))
		return domain.ErrInternal
	}
	return s.userRepo.Delete(ctx, userID)
}

// Login автентифікує користувача за email та паролем.
func (s *authService) Login(ctx context.Context, email, pass, userAgent, clientIP string, requiredRole int) (*domain.AuthResponseData, error) {
	// 1. Пошук користувача
	user, err := s.userRepo.FindByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			return nil, domain.ErrInvalidCredentials
		}
		return nil, domain.ErrInternal
	}

	// 2. Перевірка пароля
	err = password.CheckPassword(pass, user.PasswordHash)
	if err != nil {
		return nil, domain.ErrInvalidCredentials
	}

	// 2.5 Перевірка ролі (RBAC)
	if requiredRole == domain.RoleAdmin && !domain.IsAdminRole(user.RoleID) {
		return nil, domain.ErrInvalidCredentials // Приховуємо реальну причину для безпеки
	}
	// Якщо requiredRole == RoleCustomer, пускаємо і RoleCustomer, і RoleAdmin (адмін як покупець)
	if requiredRole == domain.RoleCustomer && user.RoleID != domain.RoleCustomer && !domain.IsAdminRole(user.RoleID) {
		return nil, domain.ErrForbiddenRole
	}

	// 3. Перевірка статусу верифікації (Нове правило)
	if !user.IsEmailVerified {
		return nil, domain.ErrUserNotVerified
	}

	if user.IsBlocked {
		return nil, errors.New("user account is blocked")
	}

	// 4. Генерація токенів
	return s.generateAuthSession(ctx, user, userAgent, clientIP)
}

// LoginWithGoogle перевіряє токен Google та виконує вхід або реєстрацію.
func (s *authService) LoginWithGoogle(ctx context.Context, idToken, userAgent, clientIP string) (*domain.AuthResponseData, error) {
	// 1. Верифікація токена через Google API
	// Ми передаємо GoogleClientID, щоб бібліотека перевірила, чи токен призначений саме нашому додатку (audience)
	payload, err := idtoken.Validate(ctx, idToken, s.cfg.GoogleClientID)
	if err != nil {
		s.logger.Error("failed to validate google id token", zap.Error(err))
		return nil, domain.ErrInvalidGoogleToken
	}

	// 2. Витягуємо дані з Claims
	email, ok := payload.Claims["email"].(string)
	if !ok || email == "" {
		return nil, domain.ErrInvalidGoogleToken
	}

	firstName, _ := payload.Claims["given_name"].(string)
	lastName, _ := payload.Claims["family_name"].(string)
	picture, _ := payload.Claims["picture"].(string)

	// 3. Пошук користувача за Email
	user, err := s.userRepo.FindByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			// ==========================================
			// Новий користувач -> Реєстрація
			// ==========================================
			s.logger.Info("Creating new user via Google", zap.String("email", email))

			// Генеруємо випадковий пароль, який неможливо вгадати (Security requirement)
			randomPass := s.generateNumericCode(32)
			hashedPass, _ := password.HashPassword(randomPass)

			user = &domain.User{
				ID:              uuid.New(),
				Email:           email,
				FirstName:       firstName,
				LastName:        lastName,
				PasswordHash:    hashedPass,
				RoleID:          1,    // Customer
				IsEmailVerified: true, // Google вже перевірив пошту
				IsBlocked:       false,
				AuthProvider:    "google",
				AvatarURL:       picture,
			}

			if user.Phone != nil && *user.Phone == "" {
				user.Phone = nil
			}

			if err := s.userRepo.Create(ctx, user); err != nil {
				s.logger.Error("failed to create user via google", zap.Error(err))
				return nil, domain.ErrInternal
			}
		} else {
			return nil, domain.ErrInternal
		}
	}

	// 4. Якщо користувач вже був, оновлюємо дані, якщо вони змінилися (аватар або статус верифікації)
	needsUpdate := false
	if !user.IsEmailVerified {
		user.IsEmailVerified = true
		needsUpdate = true
	}
	if picture != "" && user.AvatarURL != picture {
		user.AvatarURL = picture
		needsUpdate = true
	}

	if needsUpdate {
		_ = s.userRepo.Update(ctx, user)
	}

	if user.IsBlocked {
		return nil, errors.New("user account is blocked")
	}

	// 5. Генерація сесії
	return s.generateAuthSession(ctx, user, userAgent, clientIP)
}

// VerifyEmail підтверджує пошту кодом та повертає токени авторизації.
func (s *authService) VerifyEmail(ctx context.Context, userEmail, code string, userAgent, clientIP string) (*domain.AuthResponseData, error) {
	// 1. Знаходимо останній код
	verification, err := s.verifyCodeRepo.FindLastByTarget(ctx, userEmail, "email_verification")
	if err != nil {
		return nil, domain.ErrInternal
	}
	if verification == nil {
		return nil, domain.ErrInvalidCode
	}
	if verification.IsUsed {
		return nil, domain.ErrInvalidCode
	}

	// 2. Перевіряємо термін дії (15 хв)
	if verification.ExpiresAt.Before(time.Now()) {
		return nil, domain.ErrCodeExpired
	}

	// Перевіряємо ліміт спроб
	if verification.Attempts >= 5 {
		return nil, domain.ErrTooManyAttempts
	}

	// Перевіряємо відповідність коду
	if verification.Code != code {
		verification.Attempts++
		if updErr := s.verifyCodeRepo.UpdateAttempts(ctx, verification.ID, verification.Attempts); updErr != nil {
			s.logger.Errorw("failed to update verify code attempts", "error", updErr, "id", verification.ID)
		}
		if verification.Attempts >= 5 {
			return nil, domain.ErrTooManyAttempts
		}
		return nil, domain.ErrInvalidCode
	}

	// 3. Знаходимо користувача
	if verification.UserID == nil {
		return nil, domain.ErrInternal
	}
	user, err := s.userRepo.FindByID(ctx, *verification.UserID)
	if err != nil {
		return nil, domain.ErrInternal
	}

	// 4. Виконуємо активацію та спалення коду в одній транзакції
	err = s.userRepo.Atomic(ctx, func(uRepo domain.UserRepository, cRepo domain.VerifyCodeRepository) error {
		// Оновлюємо користувача
		user.IsEmailVerified = true
		if err := uRepo.Update(ctx, user); err != nil {
			return domain.ErrInternal
		}

		// Позначаємо код як використаний
		if err := cRepo.MarkAsUsed(ctx, verification.ID); err != nil {
			return domain.ErrInternal
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	// 5. Виконуємо вхід (Login) автоматично після підтвердження
	authData, err := s.generateAuthSession(ctx, user, userAgent, clientIP)
	if err != nil {
		return nil, err
	}

	// 6. Сповіщаємо інші пристрої (наприклад, браузер з модалкою) через WebSocket
	_ = s.wsService.NotifyUser(user.ID, map[string]interface{}{
		"type":    "EMAIL_VERIFIED",
		"payload": authData,
	})

	return authData, nil
}

// ForgotPassword ініціює процес скидання пароля.
func (s *authService) ForgotPassword(ctx context.Context, email string) error {
	// 1. Шукаємо юзера
	user, err := s.userRepo.FindByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			return nil // Не розкриваємо наявність email
		}
		return domain.ErrInternal
	}

	// 2. Генерація 6-значного коду для пароля
	code := s.generateNumericCode(6)

	// 3. Збереження коду
	vCode := &domain.VerifyCode{
		UserID:    &user.ID,
		Target:    user.Email,
		Type:      "password_reset",
		Code:      code,
		ExpiresAt: time.Now().Add(15 * time.Minute),
	}
	if err := s.verifyCodeRepo.Create(ctx, vCode); err != nil {
		return domain.ErrInternal
	}

	// 4. Відправка посилання на фронтенд
	// https://front.com/reset-password?code=123456&email=user@mail.com
	link := fmt.Sprintf("%s/reset-password?code=%s&email=%s", s.cfg.FrontendURL, code, user.Email)
	return s.emailProvider.SendPasswordResetEmail(user.Email, code, link)
}

// ResetPassword перевіряє код та встановлює новий пароль.
func (s *authService) ResetPassword(ctx context.Context, userEmail, code, newPassword string) error {
	// 1. Пошук останнього коду для скидання
	verification, err := s.verifyCodeRepo.FindLastByTarget(ctx, userEmail, "password_reset")
	if err != nil {
		return domain.ErrInternal
	}
	if verification == nil {
		return domain.ErrInvalidCode
	}
	if verification.IsUsed {
		return domain.ErrInvalidCode
	}
	if verification.ExpiresAt.Before(time.Now()) {
		return domain.ErrCodeExpired
	}

	// Перевіряємо ліміт спроб
	if verification.Attempts >= 5 {
		return domain.ErrTooManyAttempts
	}

	// Перевіряємо відповідність коду
	if verification.Code != code {
		verification.Attempts++
		if updErr := s.verifyCodeRepo.UpdateAttempts(ctx, verification.ID, verification.Attempts); updErr != nil {
			s.logger.Errorw("failed to update verify code attempts", "error", updErr, "id", verification.ID)
		}
		if verification.Attempts >= 5 {
			return domain.ErrTooManyAttempts
		}
		return domain.ErrInvalidCode
	}

	// 2. Валідація довжини пароля
	if len(newPassword) < 8 {
		return errors.New("password must be at least 8 characters long")
	}

	// 3. Перевірка, чи не є новий пароль ідентичним старому
	userForCheck, err := s.userRepo.FindByEmail(ctx, userEmail)
	if err == nil {
		err = password.CheckPassword(newPassword, userForCheck.PasswordHash)
		if err == nil {
			return domain.ErrNewPasswordSameAsOld
		}
	}

	// 4. Хешування нового пароля
	hashedPassword, err := password.HashPassword(newPassword)
	if err != nil {
		return domain.ErrInternal
	}

	// 4. Транзакційне оновлення (Atomic)
	return s.userRepo.Atomic(ctx, func(uRepo domain.UserRepository, cRepo domain.VerifyCodeRepository) error {
		// Оновлюємо пароль юзера
		user, err := uRepo.FindByEmail(ctx, userEmail)
		if err != nil {
			return domain.ErrUserNotFound
		}

		user.PasswordHash = hashedPassword
		// Якщо email ще не верифікований (наприклад, silent registration при гостьовому чекауті),
		// факт отримання email з кодом підтверджує право володіння поштою.
		if !user.IsEmailVerified {
			user.IsEmailVerified = true
		}
		if err := uRepo.Update(ctx, user); err != nil {
			return domain.ErrInternal
		}

		// Спалюємо код
		if err := cRepo.MarkAsUsed(ctx, verification.ID); err != nil {
			return domain.ErrInternal
		}

		// Видаляємо всі сесії (Security best practice)
		if err := s.sessionRepo.DeleteAllForUser(ctx, user.ID); err != nil {
			return domain.ErrInternal
		}

		return nil
	})
}

// SetupPassword перевіряє setupToken та встановлює пароль для нового користувача, після чого логінить його.
func (s *authService) SetupPassword(ctx context.Context, setupToken, newPassword, userAgent, clientIP string) (*domain.AuthResponseData, error) {
	// 1. Пошук коду для встановлення пароля
	verification, err := s.verifyCodeRepo.FindByCode(ctx, setupToken, "password_setup")
	if err != nil {
		return nil, domain.ErrInternal
	}
	if verification == nil || verification.IsUsed {
		return nil, domain.ErrInvalidCode
	}
	if verification.ExpiresAt.Before(time.Now()) {
		return nil, domain.ErrCodeExpired
	}
	if verification.UserID == nil {
		return nil, domain.ErrInternal
	}

	// 2. Валідація довжини пароля
	if len(newPassword) < 8 {
		return nil, errors.New("password must be at least 8 characters long")
	}

	// 3. Хешування нового пароля
	hashedPassword, err := password.HashPassword(newPassword)
	if err != nil {
		return nil, domain.ErrInternal
	}

	var authData *domain.AuthResponseData

	// 4. Транзакційне оновлення (Atomic)
	err = s.userRepo.Atomic(ctx, func(uRepo domain.UserRepository, cRepo domain.VerifyCodeRepository) error {
		// Оновлюємо пароль юзера
		user, err := uRepo.FindByID(ctx, *verification.UserID)
		if err != nil {
			return domain.ErrUserNotFound
		}

		user.PasswordHash = hashedPassword
		if err := uRepo.Update(ctx, user); err != nil {
			return domain.ErrInternal
		}

		// Спалюємо код
		if err := cRepo.MarkAsUsed(ctx, verification.ID); err != nil {
			return domain.ErrInternal
		}

		// Авторизуємо юзера
		authData, err = s.generateAuthSession(ctx, user, userAgent, clientIP)
		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return authData, nil
}

// ResendVerificationCode генерує та відправляє новий код.
func (s *authService) ResendVerificationCode(ctx context.Context, email string) error {
	user, err := s.userRepo.FindByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			return nil // Не розкриваємо наявність email з міркувань безпеки
		}
		return domain.ErrInternal
	}

	if user.IsEmailVerified {
		return errors.New("email is already verified")
	}

	return s.sendVerification(ctx, user)
}

// sendVerification — внутрішня логіка генерації коду та відправки листа.
func (s *authService) sendVerification(ctx context.Context, user *domain.User) error {
	// 1. Генерація 6-значного коду
	code := s.generateNumericCode(6)

	// 2. Збереження в базі
	vCode := &domain.VerifyCode{
		UserID:    &user.ID,
		Target:    user.Email,
		Type:      "email_verification",
		Code:      code,
		ExpiresAt: time.Now().Add(15 * time.Minute),
	}
	err := s.verifyCodeRepo.Create(ctx, vCode)
	if err != nil {
		return err
	}

	// 3. Формування посилання на фронтенд за новим алгоритмом
	// https://front.com/verify?code=123456&email=user@mail.com
	link := fmt.Sprintf("%s/verify?code=%s&email=%s", s.cfg.FrontendURL, code, user.Email)

	// 4. Відправка через провайдер
	return s.emailProvider.SendVerificationEmail(user.Email, code, link)
}

// generateNumericCode генерує випадковий цифровий код вказаної довжини.
func (s *authService) generateNumericCode(length int) string {
	table := [...]byte{'1', '2', '3', '4', '5', '6', '7', '8', '9', '0'}
	b := make([]byte, length)
	max := big.NewInt(int64(len(table)))
	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			panic(fmt.Sprintf("crypto rand failure: %v", err))
		}
		b[i] = table[n.Int64()]
	}
	return string(b)
}

// RequestPhoneVerification генерує код, зберігає його в базу та відправляє SMS через провайдера (Vodafone OBM).
func (s *authService) RequestPhoneVerification(ctx context.Context, phone string, userID *uuid.UUID) (string, error) {
	// Генерація 6-значного коду
	code := s.generateNumericCode(6)

	// Термін дії коду (за замовчуванням 2 хв — синхронно з validityPeriod SMS)
	ttl := s.cfg.PhoneCodeTTL
	if ttl <= 0 {
		ttl = 2 * time.Minute
	}

	// Збереження
	vCode := &domain.VerifyCode{
		UserID:    userID, // може бути nil
		Target:    phone,
		Type:      "phone_verification",
		Code:      code,
		ExpiresAt: time.Now().Add(ttl),
	}

	if err := s.verifyCodeRepo.Create(ctx, vCode); err != nil {
		s.logger.Errorw("failed to create phone verification code", "error", err, "phone", phone)
		return "", domain.ErrInternal
	}

	// Відправка SMS асинхронно: не блокуємо HTTP-відповідь і не залежимо від
	// глобального 5с request-таймауту (TimeoutMiddleware). Код уже збережено в БД,
	// тож навіть якщо провайдер тимчасово недоступний — користувач може зробити "Повторити".
	go func(phone, code string) {
		bgCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := s.smsSender.SendVerificationCode(bgCtx, phone, code); err != nil {
			s.logger.Errorw("failed to send verification SMS", "error", err, "phone", phone)
		}
	}(phone, code)

	return code, nil
}

// ConfirmPhoneVerification перевіряє код та позначає його як використаний.
func (s *authService) ConfirmPhoneVerification(ctx context.Context, phone, code string) error {
	// 1. Шукаємо останній код для цього телефону
	verification, err := s.verifyCodeRepo.FindLastByTarget(ctx, phone, "phone_verification")
	if err != nil {
		return domain.ErrInternal
	}

	if verification == nil {
		return domain.ErrInvalidCode
	}
	if verification.IsUsed {
		return domain.ErrInvalidCode
	}

	// 2. Перевіряємо час дії
	if verification.ExpiresAt.Before(time.Now()) {
		return domain.ErrCodeExpired
	}

	// Перевіряємо ліміт спроб
	if verification.Attempts >= 5 {
		return domain.ErrTooManyAttempts
	}

	// Перевіряємо відповідність коду
	if verification.Code != code {
		verification.Attempts++
		if updErr := s.verifyCodeRepo.UpdateAttempts(ctx, verification.ID, verification.Attempts); updErr != nil {
			s.logger.Errorw("failed to update verify code attempts", "error", updErr, "id", verification.ID)
		}
		if verification.Attempts >= 5 {
			return domain.ErrTooManyAttempts
		}
		return domain.ErrInvalidCode
	}

	// 3. Якщо код належить конкретному юзеру, можемо автоматично оновити його прапорець в профілі
	if verification.UserID != nil {
		err = s.userRepo.Atomic(ctx, func(uRepo domain.UserRepository, cRepo domain.VerifyCodeRepository) error {
			user, err := uRepo.FindByID(ctx, *verification.UserID)
			if err != nil {
				return err
			}

			// Якщо телефон у профілі відрізняється або не верифікований — оновлюємо
			if user.Phone == nil || *user.Phone != phone || !user.IsPhoneVerified {
				user.Phone = &phone
				user.IsPhoneVerified = true
				if err := uRepo.Update(ctx, user); err != nil {
					return err
				}
			}

			// Позначаємо код як використаний
			return cRepo.MarkAsUsed(ctx, verification.ID)
		})
	} else {
		// Якщо це гість, просто позначаємо код як використаний
		err = s.verifyCodeRepo.MarkAsUsed(ctx, verification.ID)
	}

	if err != nil {
		s.logger.Errorw("failed to confirm phone verification", "error", err, "phone", phone)
		return err
	}

	return nil
}

// Refresh оновлює пару Access + Refresh токенів за допомогою діючої сесії.
func (s *authService) Refresh(ctx context.Context, refreshToken, userAgent, clientIP string, requiredRole int) (*domain.AuthResponseData, error) {
	// 1. Знаходимо сесію за токеном
	session, err := s.sessionRepo.FindByToken(ctx, refreshToken)
	if err != nil {
		if errors.Is(err, domain.ErrSessionNotFound) {
			return nil, domain.ErrUnauthorized
		}
		return nil, domain.ErrInternal
	}

	// 2. Перевірки безпеки
	if session.IsBlocked {
		return nil, domain.ErrSessionBlocked
	}
	if session.ExpiresAt.Before(time.Now()) {
		return nil, domain.ErrSessionExpired
	}

	// 2.5 Перевірка ролі
	if requiredRole == domain.RoleAdmin && !domain.IsAdminRole(session.User.RoleID) {
		return nil, domain.ErrForbiddenRole
	}
	if requiredRole == domain.RoleCustomer && session.User.RoleID != domain.RoleCustomer && !domain.IsAdminRole(session.User.RoleID) {
		return nil, domain.ErrForbiddenRole
	}

	// 3. Refresh Token Rotation: видаляємо стару сесію
	err = s.sessionRepo.DeleteByToken(ctx, refreshToken)
	if err != nil {
		s.logger.Error("failed to delete old session during refresh", zap.Error(err))
	}

	// 4. Створюємо нову пару токенів
	return s.generateAuthSession(ctx, &session.User, userAgent, clientIP)
}

// Logout видаляє сесію користувача.
func (s *authService) Logout(ctx context.Context, refreshToken string) error {
	return s.sessionRepo.DeleteByToken(ctx, refreshToken)
}

// generateAuthSession — допоміжна функція для створення Access (JWT) та Refresh (Sessions) токенів.
func (s *authService) generateAuthSession(ctx context.Context, user *domain.User, userAgent, clientIP string) (*domain.AuthResponseData, error) {
	// 1. Конфігурація часу
	accessDuration := s.cfg.AccessTokenDuration
	refreshDuration := s.cfg.RefreshTokenDuration

	// 2. Створення Access токена
	accessToken, _, err := s.tokenMaker.CreateToken(user.ID, user.RoleID, accessDuration)
	if err != nil {
		return nil, fmt.Errorf("failed to create access token: %w", err)
	}

	// 3. Створення Refresh токена
	refreshToken, _, err := s.tokenMaker.CreateToken(user.ID, user.RoleID, refreshDuration)
	if err != nil {
		return nil, fmt.Errorf("failed to create refresh token: %w", err)
	}

	// 4. Очищення найстаріших сесій перед створенням нової (ліміт сесій)
	if err := s.sessionRepo.DeleteOldest(ctx, user.ID, s.cfg.MaxSessions-1); err != nil {
		s.logger.Error("failed to delete oldest sessions", zap.Error(err), zap.String("user_id", user.ID.String()))
		// Не перериваємо процес, якщо очищення впало
	}

	// 5. Збереження нової сесії в БД
	session := &domain.Session{
		ID:           uuid.New(),
		UserID:       user.ID,
		RefreshToken: refreshToken,
		UserAgent:    &userAgent,
		ClientIP:     &clientIP,
		IsBlocked:    false,
		ExpiresAt:    time.Now().Add(refreshDuration),
	}

	err = s.sessionRepo.Create(ctx, session)
	if err != nil {
		return nil, fmt.Errorf("failed to save session: %w", err)
	}

	return &domain.AuthResponseData{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		User:         *user,
	}, nil
}
