package http

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	validationerrors "github.com/VladHrytsaiuk/ecommerce-core/internal/shared/errors"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/user/domain"
	"go.uber.org/zap"
)

// UserHandler відповідає за операції над профілем користувача та адресами
type UserHandler struct {
	userService domain.UserService
	logger      logger.Logger
}

// NewUserHandler створює новий інстанс хендлера з необхідними залежностями
func NewUserHandler(s domain.UserService, l logger.Logger) *UserHandler {
	return &UserHandler{
		userService: s,
		logger:      l,
	}
}

// GetMe
// @Summary      Отримати профіль
// @Description  Повертає профіль поточного авторизованого користувача
// @Tags         Users
// @Security     bearerAuth
// @Produce      json
// @Success      200      {object}  UserResponse   "Профіль користувача"
// @Failure      401      {object}  ErrorResponse  "Неавторизовано"
// @Failure      404      {object}  ErrorResponse  "Користувача не знайдено"
// @Failure      500      {object}  ErrorResponse  "Внутрішня помилка сервера"
// @Router       /api/users/me [get]
func (h *UserHandler) GetMe(c *gin.Context) {
	val, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "Unauthorized",
			Message: "Authentication required. Please provide a valid token.",
		})
		return
	}
	userID, ok := val.(uuid.UUID)
	if !ok {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "Unauthorized",
			Message: "Authentication required. Please provide a valid token.",
		})
		return
	}

	user, err := h.userService.GetMe(c.Request.Context(), userID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, mapUserToResponse(*user))
}

// UpdateMe
// @Summary      Оновити профіль
// @Description  Дозволяє оновити ім'я, телефон та підписку. Використовує PATCH метод (оновлюються лише передані поля)
// @Tags         Users
// @Security     bearerAuth
// @Accept       json
// @Produce      json
// @Param        request  body      UpdateUserRequest  true  "Нові дані профілю"
// @Success      200      {object}  UserResponse       "Профіль успішно оновлено"
// @Failure      400      {object}  ErrorResponse      "Невалідні дані запиту"
// @Failure      401      {object}  ErrorResponse      "Неавторизовано"
// @Failure      404      {object}  ErrorResponse      "Користувача не знайдено"
// @Failure      500      {object}  ErrorResponse      "Внутрішня помилка сервера"
// @Router       /api/users/me [patch]
func (h *UserHandler) UpdateMe(c *gin.Context) {
	val, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "Unauthorized",
			Message: "Authentication required. Please provide a valid token.",
		})
		return
	}
	userID, ok := val.(uuid.UUID)
	if !ok {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "Unauthorized",
			Message: "Authentication required. Please provide a valid token.",
		})
		return
	}

	var req UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Invalid payload",
			Message: validationerrors.FormatValidationError(err),
		})
		return
	}

	// Мапимо вказівники у спеціальну структуру для PATCH-оновлення
	input := &domain.UpdateMeInput{
		FirstName:       req.FirstName,
		LastName:        req.LastName,
		Phone:           req.Phone,
		WantsNewsletter: req.WantsNewsletter,
	}

	updatedUser, err := h.userService.UpdateMe(c.Request.Context(), userID, input)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, mapUserToResponse(*updatedUser))
}

// DeleteMe
// @Summary      Видалити акаунт
// @Description  Виконує Soft Delete профілю користувача та анулює всі активні сесії. Дія незворотна.
// @Tags         Users
// @Security     bearerAuth
// @Produce      json
// @Success      200      {object}  MessageResponse  "Акаунт успішно видалено"
// @Failure      401      {object}  ErrorResponse    "Неавторизовано"
// @Failure      404      {object}  ErrorResponse    "Користувача не знайдено"
// @Failure      500      {object}  ErrorResponse    "Внутрішня помилка сервера"
// @Router       /api/users/me [delete]
func (h *UserHandler) DeleteMe(c *gin.Context) {
	val, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "Unauthorized",
			Message: "Authentication required. Please provide a valid token.",
		})
		return
	}
	userID, ok := val.(uuid.UUID)
	if !ok {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "Unauthorized",
			Message: "Authentication required. Please provide a valid token.",
		})
		return
	}

	err := h.userService.DeleteMe(c.Request.Context(), userID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, MessageResponse{Message: "Your account has been successfully deleted"})
}

// GetAddresses
// @Summary      Список адрес
// @Description  Повертає усі збережені адреси (Нова Пошта) юзера
// @Tags         Addresses
// @Security     bearerAuth
// @Produce      json
// @Success      200      {array}   UserAddressResponse "Список адрес"
// @Failure      401      {object}  ErrorResponse       "Неавторизовано"
// @Failure      404      {object}  ErrorResponse       "Користувача не знайдено"
// @Failure      500      {object}  ErrorResponse       "Внутрішня помилка сервера"
// @Router       /api/users/me/addresses [get]
func (h *UserHandler) GetAddresses(c *gin.Context) {
	val, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "Unauthorized",
			Message: "Authentication required. Please provide a valid token.",
		})
		return
	}
	userID, ok := val.(uuid.UUID)
	if !ok {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "Unauthorized",
			Message: "Authentication required. Please provide a valid token.",
		})
		return
	}

	addresses, err := h.userService.GetAddresses(c.Request.Context(), userID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, mapAddressesToResponse(addresses))
}

// CreateAddress
// @Summary      Додати адресу
// @Description  Додає нову адресу
// @Tags         Addresses
// @Security     bearerAuth
// @Accept       json
// @Produce      json
// @Param        request  body      CreateAddressRequest  true  "Дані нової адреси"
// @Success      201      {object}  UserAddressResponse   "Адресу успішно додано"
// @Failure      400      {object}  ErrorResponse         "Невалідні дані запиту"
// @Failure      401      {object}  ErrorResponse         "Неавторизовано"
// @Failure      404      {object}  ErrorResponse         "Користувача не знайдено"
// @Failure      500      {object}  ErrorResponse         "Внутрішня помилка сервера"
// @Router       /api/users/me/addresses [post]
func (h *UserHandler) CreateAddress(c *gin.Context) {
	val, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "Unauthorized",
			Message: "Authentication required. Please provide a valid token.",
		})
		return
	}
	userID, ok := val.(uuid.UUID)
	if !ok {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "Unauthorized",
			Message: "Authentication required. Please provide a valid token.",
		})
		return
	}

	var req CreateAddressRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Invalid payload",
			Message: validationerrors.FormatValidationError(err),
		})
		return
	}

	addressName := req.AddressName
	if addressName == "" {
		addresses, err := h.userService.GetAddresses(c.Request.Context(), userID)
		if err == nil {
			addressName = fmt.Sprintf("Адреса %d", len(addresses)+1)
		} else {
			addressName = "Моя адреса" // fallback
		}
	}

	address := &domain.UserAddress{
		UserID:        userID,
		AddressName:   addressName,
		Provider:      req.Provider,
		DeliveryType:  req.DeliveryType,
		FullAddress:   generateFullAddress(req.AreaName, req.CityName, req.WarehouseName),
		CityRef:       req.CityRef,
		CityName:      req.CityName,
		AreaRef:       req.AreaRef,
		AreaName:      req.AreaName,
		WarehouseRef:  req.WarehouseRef,
		WarehouseName: req.WarehouseName,
		IsDefault:     req.IsDefault,
	}

	created, err := h.userService.CreateAddress(c.Request.Context(), address)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusCreated, mapAddressToResponse(*created))
}

// UpdateAddress
// @Summary      Оновити адресу
// @Description  Редагує існуючу адресу (або ставить як за замовчуванням). Метод PATCH
// @Tags         Addresses
// @Security     bearerAuth
// @Accept       json
// @Produce      json
// @Param        id       path      string                true  "Address ID"
// @Param        request  body      UpdateAddressRequest  true  "Поля для оновлення"
// @Success      200      {object}  UserAddressResponse   "Адресу успішно оновлено"
// @Failure      400      {object}  ErrorResponse         "Невалідні дані запиту або ID"
// @Failure      401      {object}  ErrorResponse         "Неавторизовано"
// @Failure      404      {object}  ErrorResponse         "Адресу не знайдено"
// @Failure      500      {object}  ErrorResponse         "Внутрішня помилка сервера"
// @Router       /api/users/me/addresses/{id} [patch]
func (h *UserHandler) UpdateAddress(c *gin.Context) {
	val, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "Unauthorized",
			Message: "Authentication required. Please provide a valid token.",
		})
		return
	}
	userID, ok := val.(uuid.UUID)
	if !ok {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "Unauthorized",
			Message: "Authentication required. Please provide a valid token.",
		})
		return
	}

	idParam := c.Param("id")
	addressID, err := uuid.Parse(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Invalid address ID",
			Message: "The provided address ID is not a valid UUID.",
		})
		return
	}

	var req UpdateAddressRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Invalid payload",
			Message: validationerrors.FormatValidationError(err),
		})
		return
	}

	// Отримуємо існуючу адресу для мапінгу та перевірки прав
	// Примітка: GetAddresses повертає []domain.UserAddress, але у нас є addressID.
	// Оскільки в userService немає прямого GetAddressByID (він приватний або в репозиторії),
	// ми можемо додати його або використати існуючий репозиторій через сервіс.
	// Але зазвичай краще додати метод в сервіс.
	// Тимчасово отримуємо всі адреси користувача і шукаємо потрібну, або додаємо метод.
	// Перевіримо чи є метод GetAddresses у сервісі.

	addresses, err := h.userService.GetAddresses(c.Request.Context(), userID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	var existing *domain.UserAddress
	for i := range addresses {
		if addresses[i].ID == addressID {
			existing = &addresses[i]
			break
		}
	}

	if existing == nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "Address not found",
			Message: "The provided address ID was not found in your profile.",
		})
		return
	}

	address := &domain.UserAddress{
		ID:        existing.ID,
		UserID:    existing.UserID,
		IsDefault: existing.IsDefault,
	}

	if req.AddressName != nil {
		address.AddressName = *req.AddressName
	} else {
		address.AddressName = existing.AddressName
	}

	if req.Provider != nil {
		address.Provider = *req.Provider
	} else {
		address.Provider = existing.Provider
	}

	if req.DeliveryType != nil {
		address.DeliveryType = *req.DeliveryType
	} else {
		address.DeliveryType = existing.DeliveryType
	}

	if req.CityName != nil || req.WarehouseName != nil || req.AreaName != nil {
		// Завжди оновлюємо FullAddress, якщо змінилося місто, відділення або область
		cityName := existing.CityName
		if req.CityName != nil {
			cityName = *req.CityName
		}
		warehouseName := existing.WarehouseName
		if req.WarehouseName != nil {
			warehouseName = *req.WarehouseName
		}
		areaName := existing.AreaName
		if req.AreaName != nil {
			areaName = *req.AreaName
		}
		address.FullAddress = generateFullAddress(areaName, cityName, warehouseName)
	} else {
		address.FullAddress = existing.FullAddress
	}

	if req.CityRef != nil {
		address.CityRef = *req.CityRef
	} else {
		address.CityRef = existing.CityRef
	}

	if req.CityName != nil {
		address.CityName = *req.CityName
	} else {
		address.CityName = existing.CityName
	}

	if req.AreaRef != nil {
		address.AreaRef = *req.AreaRef
	} else {
		address.AreaRef = existing.AreaRef
	}

	if req.AreaName != nil {
		address.AreaName = *req.AreaName
	} else {
		address.AreaName = existing.AreaName
	}

	if req.WarehouseRef != nil {
		address.WarehouseRef = *req.WarehouseRef
	} else {
		address.WarehouseRef = existing.WarehouseRef
	}

	if req.WarehouseName != nil {
		address.WarehouseName = *req.WarehouseName
	} else {
		address.WarehouseName = existing.WarehouseName
	}

	if req.IsDefault != nil {
		address.IsDefault = *req.IsDefault
	}

	updated, err := h.userService.UpdateAddress(c.Request.Context(), userID, addressID, address)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, mapAddressToResponse(*updated))
}

// DeleteAddress
// @Summary      Видалити адресу
// @Description  Видаляє збережену адресу
// @Tags         Addresses
// @Security     bearerAuth
// @Produce      json
// @Param        id       path      string           true  "Address ID"
// @Success      200      {object}  MessageResponse  "Адресу видалено"
// @Failure      400      {object}  ErrorResponse    "Невалідний ID адреси"
// @Failure      401      {object}  ErrorResponse    "Неавторизовано"
// @Failure      404      {object}  ErrorResponse    "Адресу не знайдено"
// @Failure      500      {object}  ErrorResponse    "Внутрішня помилка сервера"
// @Router       /api/users/me/addresses/{id} [delete]
func (h *UserHandler) DeleteAddress(c *gin.Context) {
	val, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "Unauthorized",
			Message: "Authentication required. Please provide a valid token.",
		})
		return
	}
	userID, ok := val.(uuid.UUID)
	if !ok {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "Unauthorized",
			Message: "Authentication required. Please provide a valid token.",
		})
		return
	}

	idParam := c.Param("id")
	addressID, err := uuid.Parse(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Invalid address ID",
			Message: "The provided address ID is not a valid UUID.",
		})
		return
	}

	err = h.userService.DeleteAddress(c.Request.Context(), userID, addressID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, MessageResponse{Message: "Address deleted successfully"})
}

// SaveCheckoutAddress
// @Summary      Зберегти адресу при оформленні замовлення
// @Description  Зберігає адресу доставки під час чекауту. Назва генерується автоматично на основі міста, типу доставки та відділення.
// @Tags         Addresses
// @Security     bearerAuth
// @Accept       json
// @Produce      json
// @Param        request  body      SaveCheckoutAddressRequest  true  "Дані адреси доставки"
// @Success      201      {object}  UserAddressResponse   "Адресу успішно збережено"
// @Failure      400      {object}  ErrorResponse         "Невалідні дані запиту"
// @Failure      401      {object}  ErrorResponse         "Неавторизовано"
// @Failure      500      {object}  ErrorResponse         "Внутрішня помилка сервера"
// @Router       /api/users/me/addresses/checkout [post]
func (h *UserHandler) SaveCheckoutAddress(c *gin.Context) {
	val, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "Unauthorized",
			Message: "Authentication required. Please provide a valid token.",
		})
		return
	}
	userID, ok := val.(uuid.UUID)
	if !ok {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "Unauthorized",
			Message: "Authentication required. Please provide a valid token.",
		})
		return
	}

	var req SaveCheckoutAddressRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Invalid payload",
			Message: validationerrors.FormatValidationError(err),
		})
		return
	}

	// Отримуємо існуючі адреси для визначення порядкового номера
	addresses, err := h.userService.GetAddresses(c.Request.Context(), userID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	// Перевіряємо чи така адреса вже існує
	for _, addr := range addresses {
		if addr.Provider == req.Provider &&
			addr.DeliveryType == req.DeliveryType &&
			addr.CityRef == req.CityRef &&
			addr.WarehouseRef == req.WarehouseRef {
			// Адреса вже існує, повертаємо її без створення дубліката
			c.JSON(http.StatusOK, mapAddressToResponse(addr))
			return
		}
	}

	// Автоматична генерація простої назви: "Адреса 1", "Адреса 2" і т.д.
	addressName := fmt.Sprintf("Адреса %d", len(addresses)+1)

	// Якщо це перша адреса, робимо її дефолтною
	isDefault := len(addresses) == 0

	address := &domain.UserAddress{
		UserID:        userID,
		AddressName:   addressName,
		Provider:      req.Provider,
		DeliveryType:  req.DeliveryType,
		FullAddress:   generateFullAddress(req.AreaName, req.CityName, req.WarehouseName),
		CityRef:       req.CityRef,
		CityName:      req.CityName,
		AreaRef:       req.AreaRef,
		AreaName:      req.AreaName,
		WarehouseRef:  req.WarehouseRef,
		WarehouseName: req.WarehouseName,
		IsDefault:     isDefault,
	}

	created, err := h.userService.CreateAddress(c.Request.Context(), address)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusCreated, mapAddressToResponse(*created))
}

// generateAddressName формує людиночитабельну назву адреси з компонентів.
// Результат: "Львів. Поштомат \"Нова Пошта\" №36057: вул. Симона Петлюри, 2"
func generateAddressName(cityName, provider, deliveryType, warehouseName string) string {
	// Переклад provider на людську назву
	providerLabel := translateProvider(provider)

	// Переклад типу доставки
	deliveryLabel := translateDeliveryType(deliveryType)

	// Формат: "{Місто}. {Тип доставки} \"{Провайдер}\" {Назва відділення}"
	return fmt.Sprintf("%s. %s \"%s\" %s", cityName, deliveryLabel, providerLabel, warehouseName)
}

// generateFullAddress формує повну адресу за шаблоном: "{AreaName} область, м. {CityName}, {WarehouseName}"
func generateFullAddress(areaName, cityName, warehouseName string) string {
	if areaName != "" {
		return fmt.Sprintf("%s область, м. %s, %s", areaName, cityName, warehouseName)
	}
	return fmt.Sprintf("м. %s, %s", cityName, warehouseName)
}

// translateProvider перетворює код провайдера в людську назву.
func translateProvider(provider string) string {
	switch strings.ToUpper(provider) {
	case "NOVA_POSHTA":
		return "Нова Пошта"
	case "UKR_POSHTA":
		return "Укрпошта"
	case "MEEST":
		return "Meest"
	default:
		return provider
	}
}

// translateDeliveryType перетворює код типу доставки в людську назву.
func translateDeliveryType(deliveryType string) string {
	switch strings.ToUpper(deliveryType) {
	case "BRANCH":
		return "Відділення"
	case "POSTOMAT":
		return "Поштомат"
	case "COURIER":
		return "Кур'єрська доставка"
	default:
		return deliveryType
	}
}

// handleError мапує доменні помилки у відповідні HTTP статуси
func (h *UserHandler) handleError(c *gin.Context, err error) {
	if errors.Is(err, domain.ErrUserNotFound) || errors.Is(err, domain.ErrSessionNotFound) {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "Not Found",
			Message: err.Error(),
		})
		return
	}
	if errors.Is(err, domain.ErrUnauthorized) {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "Unauthorized",
			Message: err.Error(),
		})
		return
	}

	h.logger.Error("unexpected internal error", zap.Error(err))
	c.JSON(http.StatusInternalServerError, ErrorResponse{
		Error:   "Internal Server Error",
		Message: "An unexpected error occurred. Please try again later.",
	})
}

func mapUserToResponse(u domain.User) UserResponse {
	return UserResponse{
		ID:              u.ID.String(),
		RoleID:          u.RoleID,
		FirstName:       u.FirstName,
		LastName:        u.LastName,
		Email:           u.Email,
		Phone:           u.Phone,
		IsEmailVerified: u.IsEmailVerified,
		IsPhoneVerified: u.IsPhoneVerified,
		WantsNewsletter: u.WantsNewsletter,
		AuthProvider:    u.AuthProvider,
		AvatarURL:       u.AvatarURL,
	}
}

func mapAddressToResponse(a domain.UserAddress) UserAddressResponse {
	return UserAddressResponse{
		ID:            a.ID.String(),
		AddressName:   a.AddressName,
		Provider:      a.Provider,
		DeliveryType:  a.DeliveryType,
		FullAddress:   a.FullAddress,
		CityRef:       a.CityRef,
		CityName:      a.CityName,
		AreaRef:       a.AreaRef,
		AreaName:      a.AreaName,
		WarehouseRef:  a.WarehouseRef,
		WarehouseName: a.WarehouseName,
		IsDefault:     a.IsDefault,
	}
}

func mapAddressesToResponse(addresses []domain.UserAddress) []UserAddressResponse {
	res := make([]UserAddressResponse, len(addresses))
	for i, a := range addresses {
		res[i] = mapAddressToResponse(a)
	}
	return res
}
