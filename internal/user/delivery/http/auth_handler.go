package http

import (
	"errors"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	validationerrors "github.com/VladHrytsaiuk/ecommerce-core/internal/shared/errors"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/user/domain"
	"go.uber.org/zap"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/config"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/notification"
)

// AuthHandler відповідає за обробку HTTP-запитів по аутентифікації
type AuthHandler struct {
	authService domain.AuthService
	wsHub       *notification.Hub
	cfg         *config.Config
	logger      logger.Logger
	upgrader    websocket.Upgrader
}

// NewAuthHandler створює новий інстанс хендлера з необхідними залежностями
func NewAuthHandler(s domain.AuthService, hub *notification.Hub, cfg *config.Config, l logger.Logger) *AuthHandler {
	return &AuthHandler{
		authService: s,
		wsHub:       hub,
		cfg:         cfg,
		logger:      l,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				origin := r.Header.Get("Origin")
				if origin == "" || origin == "null" {
					return true
				}

				// Allow same origin
				host := r.Host
				if origin == "http://"+host || origin == "https://"+host {
					return true
				}

				// Normalize origins
				normalize := func(s string) string {
					if len(s) > 0 && s[len(s)-1] == '/' {
						return s[:len(s)-1]
					}
					return s
				}

				normalizedOrigin := normalize(origin)
				allowedFrontend := normalize(cfg.FrontendURL)

				// Allow configured frontend
				if normalizedOrigin == allowedFrontend {
					return true
				}

				// Allow local development on any common port
				if normalizedOrigin == "http://localhost:3000" ||
					normalizedOrigin == "http://127.0.0.1:3000" ||
					normalizedOrigin == "http://localhost:8080" ||
					normalizedOrigin == "http://127.0.0.1:8080" {
					return true
				}

				l.Warnw("WebSocket origin not allowed",
					"origin", origin,
					"allowed_frontend", allowedFrontend,
					"host", host,
				)
				return false
			},
		},
	}
}

// Register
// @Summary      Реєстрація користувача
// @Description  Створює новий акаунт за email та паролем. Повертає токени для входу.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        request  body      RegisterRequest   true  "Дані для реєстрації"
// @Success      201      {object}  RegisterResponse  "Успішна реєстрація"
// @Failure      400      {object}  ErrorResponse     "Невалідні дані (validation error)"
// @Failure      409      {object}  ErrorResponse     "Користувач з таким email вже існує"
// @Failure      500      {object}  ErrorResponse     "Внутрішня помилка сервера"
// @Router       /api/auth/register [post]
func (h *AuthHandler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Invalid request payload",
			Message: validationerrors.FormatValidationError(err),
		})
		return
	}

	user := &domain.User{
		Email:        req.Email,
		PasswordHash: req.Password,
		FirstName:    req.FirstName,
		LastName:     req.LastName,
	}

	authData, err := h.authService.Register(c.Request.Context(), user)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusCreated, RegisterResponse{
		Message: "Registration successful. Please check your email for verification link.",
		User:    mapUserToResponse(authData.User),
	})
}

// CreateAdmin
// @Summary      Створити адміністратора або Власника
// @Description  Створює активний акаунт з початковим паролем без email-запрошення.
// @Tags         Admin Users
// @Security     bearerAuth
// @Accept       json
// @Produce      json
// @Param        request body CreateAdminRequest true "Дані адміністратора"
// @Success      201 {object} UserResponse
// @Router       /api/admin/users [post]
func (h *AuthHandler) CreateAdmin(c *gin.Context) {
	var req CreateAdminRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid request payload", Message: validationerrors.FormatValidationError(err)})
		return
	}

	user, err := h.authService.CreateAdmin(c.Request.Context(), &domain.User{
		FirstName:    req.FirstName,
		LastName:     req.LastName,
		Email:        req.Email,
		PasswordHash: req.Password,
		RoleID:       req.RoleID,
	})
	if err != nil {
		h.handleError(c, err)
		return
	}
	c.JSON(http.StatusCreated, mapUserToResponse(*user))
}

// ListAdmins
// @Summary      Список адміністраторів
// @Tags         Admin Users
// @Security     bearerAuth
// @Produce      json
// @Success      200 {array} UserResponse
// @Router       /api/admin/users [get]
func (h *AuthHandler) ListAdmins(c *gin.Context) {
	users, err := h.authService.ListAdmins(c.Request.Context())
	if err != nil {
		h.handleError(c, err)
		return
	}
	result := make([]UserResponse, len(users))
	for i, user := range users {
		result[i] = mapUserToResponse(user)
	}
	c.JSON(http.StatusOK, result)
}

// DeleteAdmin
// @Summary      Видалити адміністратора
// @Description  Доступно лише Власнику. Видаляє звичайного адміністратора та завершує його сесії.
// @Tags         Admin Users
// @Security     bearerAuth
// @Success      204
// @Router       /api/admin/users/{id} [delete]
func (h *AuthHandler) DeleteAdmin(c *gin.Context) {
	actor, exists := c.Get("user_id")
	actorID, ok := actor.(uuid.UUID)
	if !exists || !ok {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "Unauthorized", Message: "Authentication required."})
		return
	}
	userID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid user id", Message: "Некоректний ідентифікатор користувача."})
		return
	}
	if err := h.authService.DeleteAdmin(c.Request.Context(), actorID, userID); err != nil {
		h.handleError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// Login
// @Summary      Логін
// @Description  Повертає пару access/refresh токенів (JWT)
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        request  body      LoginRequest  true  "Дані для входу"
// @Success      200      {object}  AuthResponse  "Успішний вхід"
// @Failure      400      {object}  ErrorResponse "Невалідний запит"
// @Failure      401      {object}  ErrorResponse "Невірний email або пароль"
// @Failure      403      {object}  ErrorResponse "Акаунт заблоковано або не верифіковано"
// @Failure      500      {object}  ErrorResponse "Внутрішня помилка сервера"
// @Router       /api/auth/login [post]
func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Invalid request payload",
			Message: validationerrors.FormatValidationError(err),
		})
		return
	}

	authData, err := h.authService.Login(
		c.Request.Context(),
		req.Email,
		req.Password,
		c.Request.UserAgent(),
		c.ClientIP(),
		domain.RoleCustomer, // Клієнтський логін
	)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, AuthResponse{
		AccessToken:  authData.AccessToken,
		RefreshToken: authData.RefreshToken,
		User:         mapUserToResponse(authData.User),
	})
}

// AdminLogin
// @Summary      Логін для адміністраторів
// @Description  Логін виключно для ролі Admin
// @Tags         Admin Auth
// @Accept       json
// @Produce      json
// @Param        request  body      LoginRequest  true  "Дані для входу"
// @Success      200      {object}  AuthResponse  "Успішний вхід"
// @Failure      400      {object}  ErrorResponse "Невалідний запит"
// @Failure      401      {object}  ErrorResponse "Невірний email або пароль"
// @Failure      403      {object}  ErrorResponse "Заборонено"
// @Failure      500      {object}  ErrorResponse "Внутрішня помилка сервера"
// @Router       /api/admin/auth/login [post]
func (h *AuthHandler) AdminLogin(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Invalid request payload",
			Message: validationerrors.FormatValidationError(err),
		})
		return
	}

	authData, err := h.authService.Login(
		c.Request.Context(),
		req.Email,
		req.Password,
		c.Request.UserAgent(),
		c.ClientIP(),
		domain.RoleAdmin, // Тільки адміни
	)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, AuthResponse{
		AccessToken:  authData.AccessToken,
		RefreshToken: authData.RefreshToken,
		User:         mapUserToResponse(authData.User),
	})
}

// LoginWithGoogle
// @Summary      Вхід через Google
// @Description  Приймає ID Token від Google, верифікує його та повертає JWT токени. Автоматично реєструє користувача, якщо його немає в системі.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        request  body      GoogleLoginRequest  true  "Google ID Token"
// @Success      200      {object}  AuthResponse        "Успішний вхід"
// @Failure      400      {object}  ErrorResponse       "Невалідний запит"
// @Failure      401      {object}  ErrorResponse       "Невалідний Google токен"
// @Failure      403      {object}  ErrorResponse       "Акаунт заблоковано або не верифіковано"
// @Failure      500      {object}  ErrorResponse       "Внутрішня помилка сервера"
// @Router       /api/auth/google [post]
func (h *AuthHandler) LoginWithGoogle(c *gin.Context) {
	var req GoogleLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Invalid request payload",
			Message: validationerrors.FormatValidationError(err),
		})
		return
	}

	authData, err := h.authService.LoginWithGoogle(
		c.Request.Context(),
		req.IDToken,
		c.Request.UserAgent(),
		c.ClientIP(),
	)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, AuthResponse{
		AccessToken:  authData.AccessToken,
		RefreshToken: authData.RefreshToken,
		User:         mapUserToResponse(authData.User),
	})
}

// Refresh
// @Summary      Оновлення токенів
// @Description  Повертає нові access/refresh токени за валідним старим refresh_token
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        request  body      RefreshRequest  true  "Refresh токен"
// @Success      200      {object}  AuthResponse    "Нові токени успішно згенеровані"
// @Failure      400      {object}  ErrorResponse   "Токен обов'язковий"
// @Failure      401      {object}  ErrorResponse   "Refresh токен недійсний або прострочений"
// @Failure      403      {object}  ErrorResponse   "Сесія заблокована або прострочена"
// @Failure      500      {object}  ErrorResponse   "Внутрішня помилка сервера"
// @Router       /api/auth/refresh [post]
func (h *AuthHandler) Refresh(c *gin.Context) {
	var req RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Refresh token is required",
			Message: validationerrors.FormatValidationError(err),
		})
		return
	}

	authData, err := h.authService.Refresh(
		c.Request.Context(),
		req.RefreshToken,
		c.Request.UserAgent(),
		c.ClientIP(),
		domain.RoleCustomer,
	)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, AuthResponse{
		AccessToken:  authData.AccessToken,
		RefreshToken: authData.RefreshToken,
		User:         mapUserToResponse(authData.User),
	})
}

// AdminRefresh
// @Summary      Оновлення токенів для адміністраторів
// @Description  Повертає нові access/refresh токени за валідним старим refresh_token з перевіркою ролі Admin
// @Tags         Admin Auth
// @Accept       json
// @Produce      json
// @Param        request  body      RefreshRequest  true  "Refresh токен"
// @Success      200      {object}  AuthResponse    "Нові токени успішно згенеровані"
// @Failure      400      {object}  ErrorResponse   "Токен обов'язковий"
// @Failure      401      {object}  ErrorResponse   "Refresh токен недійсний або прострочений"
// @Failure      403      {object}  ErrorResponse   "Заборонено"
// @Failure      500      {object}  ErrorResponse   "Внутрішня помилка сервера"
// @Router       /api/admin/auth/refresh [post]
func (h *AuthHandler) AdminRefresh(c *gin.Context) {
	var req RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Refresh token is required",
			Message: validationerrors.FormatValidationError(err),
		})
		return
	}

	authData, err := h.authService.Refresh(
		c.Request.Context(),
		req.RefreshToken,
		c.Request.UserAgent(),
		c.ClientIP(),
		domain.RoleAdmin,
	)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, AuthResponse{
		AccessToken:  authData.AccessToken,
		RefreshToken: authData.RefreshToken,
		User:         mapUserToResponse(authData.User),
	})
}

// Logout
// @Summary      Вихід (Logout)
// @Description  Інвалідує поточну сесію. Використовує RefreshToken з тіла запиту.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        request  body      RefreshRequest  true  "Refresh токен для видалення сесії"
// @Success      200      {object}  MessageResponse "Успішний вихід"
// @Failure      400      {object}  ErrorResponse   "Токен обов'язковий"
// @Failure      401      {object}  ErrorResponse   "Неавторизовано"
// @Failure      500      {object}  ErrorResponse   "Внутрішня помилка сервера"
// @Router       /api/auth/logout [post]
func (h *AuthHandler) Logout(c *gin.Context) {
	var req RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Refresh token is required",
			Message: validationerrors.FormatValidationError(err),
		})
		return
	}

	err := h.authService.Logout(c.Request.Context(), req.RefreshToken)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, MessageResponse{Message: "Successfully logged out"})
}

// VerifyEmail
// @Summary      Підтвердження email
// @Description  Перевіряє OTP код для верифікації email. Повертає токени авторизації при успіху.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        request  body      VerifyEmailRequest  true  "Email та код підтвердження"
// @Success      200      {object}  AuthResponse        "Email успішно підтверджено, вхід виконано"
// @Failure      400      {object}  ErrorResponse       "Невалідний запит або код"
// @Failure      401      {object}  ErrorResponse       "Неавторизовано"
// @Failure      500      {object}  ErrorResponse       "Внутрішня помилка сервера"
// @Router       /api/auth/verify-email [post]
func (h *AuthHandler) VerifyEmail(c *gin.Context) {
	var req VerifyEmailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Invalid request payload",
			Message: validationerrors.FormatValidationError(err),
		})
		return
	}

	authData, err := h.authService.VerifyEmail(
		c.Request.Context(),
		req.Email,
		req.Code,
		c.Request.UserAgent(),
		c.ClientIP(),
	)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, AuthResponse{
		AccessToken:  authData.AccessToken,
		RefreshToken: authData.RefreshToken,
		User:         mapUserToResponse(authData.User),
	})
}

// ResendVerification
// @Summary      Повторна відправка коду
// @Description  Генерує та відправляє новий код верифікації на вказану пошту
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        request  body      ForgotPasswordRequest  true  "Email користувача"
// @Success      200      {object}  MessageResponse        "Код успішно відправлено"
// @Failure      400      {object}  ErrorResponse          "Невалідний запит"
// @Failure      401      {object}  ErrorResponse          "Неавторизовано"
// @Failure      404      {object}  ErrorResponse          "Користувача не знайдено"
// @Failure      500      {object}  ErrorResponse          "Внутрішня помилка сервера"
// @Router       /api/auth/resend-verification [post]
func (h *AuthHandler) ResendVerification(c *gin.Context) {
	var req ForgotPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Invalid request payload",
			Message: validationerrors.FormatValidationError(err),
		})
		return
	}

	err := h.authService.ResendVerificationCode(c.Request.Context(), req.Email)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, MessageResponse{Message: "Verification email sent successfully"})
}

// ForgotPassword
// @Summary      Запит на скидання пароля
// @Description  Генерує код для скидання пароля та відправляє його на вказаний email.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        request  body      ForgotPasswordRequest  true  "Email користувача"
// @Success      200      {object}  MessageResponse        "Лист із кодом успішно відправлено"
// @Failure      400      {object}  ErrorResponse          "Невалідний email"
// @Failure      404      {object}  ErrorResponse          "Користувача не знайдено"
// @Failure      500      {object}  ErrorResponse          "Внутрішня помилка сервера"
// @Router       /api/auth/forgot-password [post]
func (h *AuthHandler) ForgotPassword(c *gin.Context) {
	var req ForgotPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Invalid request payload",
			Message: validationerrors.FormatValidationError(err),
		})
		return
	}

	err := h.authService.ForgotPassword(c.Request.Context(), req.Email)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, MessageResponse{Message: "Password reset email sent successfully"})
}

// ResetPassword
// @Summary      Зміна пароля за кодом
// @Description  Встановлює новий пароль для користувача після перевірки OTP коду.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        request  body      ResetPasswordRequest  true  "Email, код та новий пароль"
// @Success      200      {object}  MessageResponse       "Пароль успішно змінено"
// @Failure      400      {object}  ErrorResponse         "Невалідний код або пароль"
// @Failure      500      {object}  ErrorResponse         "Внутрішня помилка сервера"
// @Router       /api/auth/reset-password [post]
func (h *AuthHandler) ResetPassword(c *gin.Context) {
	var req ResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Invalid request payload",
			Message: validationerrors.FormatValidationError(err),
		})
		return
	}

	err := h.authService.ResetPassword(c.Request.Context(), req.Email, req.Code, req.NewPassword)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, MessageResponse{Message: "Password has been reset successfully"})
}

// SetupPassword
// @Summary      Встановлення пароля для нового користувача
// @Description  Встановлює пароль для юзера, створеного під час гостьового чекауту, та логінить його.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        request  body      SetupPasswordRequest  true  "Токен та новий пароль"
// @Success      200      {object}  AuthResponse          "Пароль успішно задано, вхід виконано"
// @Failure      400      {object}  ErrorResponse         "Невалідний запит"
// @Failure      500      {object}  ErrorResponse         "Внутрішня помилка сервера"
// @Router       /api/auth/setup-password [post]
func (h *AuthHandler) SetupPassword(c *gin.Context) {
	var req SetupPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Invalid request payload",
			Message: validationerrors.FormatValidationError(err),
		})
		return
	}

	authData, err := h.authService.SetupPassword(
		c.Request.Context(),
		req.SetupToken,
		req.Password,
		c.Request.UserAgent(),
		c.ClientIP(),
	)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, AuthResponse{
		AccessToken:  authData.AccessToken,
		RefreshToken: authData.RefreshToken,
		User:         mapUserToResponse(authData.User),
	})
}

// RequestPhoneVerification
// @Summary      Запит на верифікацію телефону (SMS)
// @Description  Генерує OTP-код та логує його (емуляція відправки SMS).
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        request  body      VerifyPhoneRequest   true  "Номер телефону"
// @Success      200      {object}  VerifyPhoneResponse  "Код згенеровано"
// @Router       /api/auth/verify-phone/request [post]
func (h *AuthHandler) RequestPhoneVerification(c *gin.Context) {
	var req VerifyPhoneRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Invalid request payload",
			Message: validationerrors.FormatValidationError(err),
		})
		return
	}

	var userIDPtr *uuid.UUID
	if userIDRaw, exists := c.Get("user_id"); exists {
		switch v := userIDRaw.(type) {
		case string:
			id, _ := uuid.Parse(v)
			userIDPtr = &id
		case uuid.UUID:
			userIDPtr = &v
		}
	}

	code, err := h.authService.RequestPhoneVerification(c.Request.Context(), req.Phone, userIDPtr)
	if err != nil {
		h.handleError(c, err)
		return
	}

	// У dev-оточенні повертаємо код прямо в JSON, щоб полегшити тестування.
	// Безпечно за замовчуванням: код віддається ЛИШЕ за явного APP_ENV=development|local.
	// Будь-яке інше або не задане значення (зокрема прод) — код іде тільки через SMS.
	resp := VerifyPhoneResponse{
		Message: "Verification code sent successfully",
	}
	env := os.Getenv("APP_ENV")
	if env == "development" || env == "local" {
		resp.Code = code
	}

	c.JSON(http.StatusOK, resp)
}

// ConfirmPhoneVerification
// @Summary      Підтвердження телефону
// @Description  Перевіряє OTP-код. Якщо користувач авторизований, оновлює його профіль.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        request  body      ConfirmPhoneRequest  true  "Номер телефону та код"
// @Success      200      {object}  MessageResponse      "Телефон успішно підтверджено"
// @Router       /api/auth/verify-phone/confirm [post]
func (h *AuthHandler) ConfirmPhoneVerification(c *gin.Context) {
	var req ConfirmPhoneRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Invalid request payload",
			Message: validationerrors.FormatValidationError(err),
		})
		return
	}

	err := h.authService.ConfirmPhoneVerification(c.Request.Context(), req.Phone, req.Code)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, MessageResponse{Message: "Phone verified successfully"})
}

// WSConnect встановлює WebSocket з'єднання для отримання сповіщень.
func (h *AuthHandler) WSConnect(c *gin.Context) {
	userIDStr := c.Query("user_id")
	if userIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id query parameter is required"})
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user_id format"})
		return
	}

	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.logger.Errorw("Failed to upgrade connection to WebSocket", "error", err, "user_id", userID)
		return
	}

	// Створюємо нового клієнта та реєструємо його в хабі
	client := notification.NewClient(h.wsHub, conn, userID)
	h.wsHub.RegisterClient(client)

	// Запускаємо відправку та читання в окремих горутинах
	go client.WritePump()
	go client.ReadPump()
}

// handleError мапує доменні помилки у відповідні HTTP статуси
func (h *AuthHandler) handleError(c *gin.Context, err error) {
	if errors.Is(err, domain.ErrUserNotFound) {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "Not Found",
			Message: err.Error(),
		})
		return
	}
	if errors.Is(err, domain.ErrInvalidCredentials) ||
		errors.Is(err, domain.ErrUnauthorized) ||
		errors.Is(err, domain.ErrInvalidGoogleToken) {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "Unauthorized",
			Message: err.Error(),
		})
		return
	}
	if errors.Is(err, domain.ErrEmailAlreadyExists) || errors.Is(err, domain.ErrPhoneAlreadyExists) {
		c.JSON(http.StatusConflict, ErrorResponse{
			Error:   "Conflict",
			Message: err.Error(),
		})
		return
	}
	if errors.Is(err, domain.ErrSessionBlocked) || errors.Is(err, domain.ErrSessionExpired) || errors.Is(err, domain.ErrForbiddenRole) {
		c.JSON(http.StatusForbidden, ErrorResponse{
			Error:   "Forbidden",
			Message: err.Error(),
		})
		return
	}
	if errors.Is(err, domain.ErrUserNotVerified) {
		c.JSON(http.StatusForbidden, ErrorResponse{
			Error:   "Forbidden",
			Message: "Please verify your email address to access the system",
		})
		return
	}
	if errors.Is(err, domain.ErrInvalidCode) {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "INVALID_CODE",
			Message: "Невірний код. Спробуйте ще раз",
		})
		return
	}
	if errors.Is(err, domain.ErrTooManyAttempts) {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "TOO_MANY_VERIFY_ATTEMPTS",
			Message: "Вичерпано ліміт спроб. Отримайте новий код.",
		})
		return
	}
	if errors.Is(err, domain.ErrCodeExpired) {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "CODE_EXPIRED",
			Message: "Код підтвердження прострочений",
		})
		return
	}

	h.logger.Error("unexpected internal error", zap.Error(err))
	c.JSON(http.StatusInternalServerError, ErrorResponse{
		Error:   "Internal Server Error",
		Message: "An unexpected error occurred. Please try again later.",
	})
}
