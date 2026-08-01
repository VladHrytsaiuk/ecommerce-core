package http

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/user/domain"
)

func ptr[T any](v T) *T {
	return &v
}

// newUserRouter — допоміжна функція для тестів UserHandler.
// injectUserID вставляє user_id у контекст Gin перед хендлером (замінює AuthMiddleware).
func newUserRouter(svc *MockUserService, userID uuid.UUID) *gin.Engine {
	r := gin.New()
	h := NewUserHandler(svc, &noopLogger{})

	inject := func(c *gin.Context) {
		c.Set("user_id", userID)
		c.Next()
	}

	r.GET("/users/me", inject, h.GetMe)
	r.PATCH("/users/me", inject, h.UpdateMe)
	r.DELETE("/users/me", inject, h.DeleteMe)
	r.GET("/users/me/addresses", inject, h.GetAddresses)
	r.POST("/users/me/addresses", inject, h.CreateAddress)
	r.PATCH("/users/me/addresses/:id", inject, h.UpdateAddress)
	r.DELETE("/users/me/addresses/:id", inject, h.DeleteAddress)
	return r
}

// newUserRouterNoAuth — роутер без user_id в контексті (для тестів авторизації)
func newUserRouterNoAuth(svc *MockUserService) *gin.Engine {
	r := gin.New()
	h := NewUserHandler(svc, &noopLogger{})
	r.GET("/users/me", h.GetMe)
	r.PATCH("/users/me", h.UpdateMe)
	r.DELETE("/users/me", h.DeleteMe)
	r.GET("/users/me/addresses", h.GetAddresses)
	r.POST("/users/me/addresses", h.CreateAddress)
	return r
}

func makeTestUser(userID uuid.UUID) *domain.User {
	return &domain.User{
		ID:              userID,
		RoleID:          1,
		FirstName:       "John",
		LastName:        "Doe",
		Email:           "john@example.com",
		Phone:           ptr("+380501234567"),
		IsEmailVerified: true,
		PasswordHash:    "secret-bcrypt-hash", // не повинно з'явитися у відповіді
	}
}

func makeTestAddressHTTP(userID uuid.UUID) *domain.UserAddress {
	return &domain.UserAddress{
		ID:            uuid.New(),
		UserID:        userID,
		Provider:      "NOVA_POSHTA",
		DeliveryType:  "BRANCH",
		FullAddress:   "Київська область, м. Київ, Відділення №5",
		CityRef:       "city-ref-123",
		CityName:      "Київ",
		AreaRef:       "area-ref-789",
		AreaName:      "Київська",
		WarehouseRef:  "wh-ref-456",
		WarehouseName: "Відділення №5",
		IsDefault:     true,
	}
}

// ==========================================
// GetMe
// ==========================================

func TestUserHandler_GetMe_Success(t *testing.T) {
	svc := &MockUserService{}
	userID := uuid.New()
	router := newUserRouter(svc, userID)

	user := makeTestUser(userID)
	svc.On("GetMe", mock.Anything, userID).Return(user, nil)

	req, _ := http.NewRequest(http.MethodGet, "/users/me", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp UserResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "john@example.com", resp.Email)
	assert.Equal(t, "John", resp.FirstName)
}

func TestUserHandler_GetMe_PasswordNotInResponse(t *testing.T) {
	svc := &MockUserService{}
	userID := uuid.New()
	router := newUserRouter(svc, userID)

	user := makeTestUser(userID)
	svc.On("GetMe", mock.Anything, userID).Return(user, nil)

	req, _ := http.NewRequest(http.MethodGet, "/users/me", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// password_hash ніколи не повинен з'являтися у відповіді
	assert.NotContains(t, w.Body.String(), "secret-bcrypt-hash")
	assert.NotContains(t, w.Body.String(), "password_hash")
}

func TestUserHandler_GetMe_NoUserIDInContext(t *testing.T) {
	svc := &MockUserService{}
	router := newUserRouterNoAuth(svc)

	req, _ := http.NewRequest(http.MethodGet, "/users/me", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestUserHandler_GetMe_UserNotFound(t *testing.T) {
	svc := &MockUserService{}
	userID := uuid.New()
	router := newUserRouter(svc, userID)

	svc.On("GetMe", mock.Anything, userID).Return(nil, domain.ErrUserNotFound)

	req, _ := http.NewRequest(http.MethodGet, "/users/me", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ==========================================
// UpdateMe
// ==========================================

func TestUserHandler_UpdateMe_Success(t *testing.T) {
	svc := &MockUserService{}
	userID := uuid.New()
	router := newUserRouter(svc, userID)

	updated := makeTestUser(userID)
	updated.FirstName = "Updated"
	svc.On("UpdateMe", mock.Anything, userID, mock.Anything).Return(updated, nil)

	body := bytes.NewBufferString(`{"first_name": "Updated"}`)
	req, _ := http.NewRequest(http.MethodPatch, "/users/me", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp UserResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "Updated", resp.FirstName)
}

func TestUserHandler_UpdateMe_NoUserIDInContext(t *testing.T) {
	svc := &MockUserService{}
	router := newUserRouterNoAuth(svc)

	body := bytes.NewBufferString(`{"first_name": "Updated"}`)
	req, _ := http.NewRequest(http.MethodPatch, "/users/me", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestUserHandler_UpdateMe_InvalidJSON(t *testing.T) {
	svc := &MockUserService{}
	userID := uuid.New()
	router := newUserRouter(svc, userID)

	req, _ := http.NewRequest(http.MethodPatch, "/users/me", bytes.NewBufferString("not-json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ==========================================
// DeleteMe
// ==========================================

func TestUserHandler_DeleteMe_Success(t *testing.T) {
	svc := &MockUserService{}
	userID := uuid.New()
	router := newUserRouter(svc, userID)

	svc.On("DeleteMe", mock.Anything, userID).Return(nil)

	req, _ := http.NewRequest(http.MethodDelete, "/users/me", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp MessageResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Contains(t, resp.Message, "deleted")
}

func TestUserHandler_DeleteMe_NoUserIDInContext(t *testing.T) {
	svc := &MockUserService{}
	router := newUserRouterNoAuth(svc)

	req, _ := http.NewRequest(http.MethodDelete, "/users/me", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// ==========================================
// GetAddresses
// ==========================================

func TestUserHandler_GetAddresses_Success(t *testing.T) {
	svc := &MockUserService{}
	userID := uuid.New()
	router := newUserRouter(svc, userID)

	addresses := []domain.UserAddress{
		*makeTestAddressHTTP(userID),
		*makeTestAddressHTTP(userID),
	}
	svc.On("GetAddresses", mock.Anything, userID).Return(addresses, nil)

	req, _ := http.NewRequest(http.MethodGet, "/users/me/addresses", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp []UserAddressResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Len(t, resp, 2)
}

func TestUserHandler_GetAddresses_NoUserIDInContext(t *testing.T) {
	svc := &MockUserService{}
	router := newUserRouterNoAuth(svc)

	req, _ := http.NewRequest(http.MethodGet, "/users/me/addresses", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// ==========================================
// CreateAddress
// ==========================================

func TestUserHandler_CreateAddress_Success(t *testing.T) {
	svc := &MockUserService{}
	userID := uuid.New()
	router := newUserRouter(svc, userID)

	created := makeTestAddressHTTP(userID)
	// Хендлер тепер викликає GetAddresses для генерації назви, якщо вона не передана
	svc.On("GetAddresses", mock.Anything, userID).Return([]domain.UserAddress{}, nil)
	svc.On("CreateAddress", mock.Anything, mock.Anything).Return(created, nil)

	body := toJSON(t, map[string]interface{}{
		"provider":       "NOVA_POSHTA",
		"delivery_type":  "BRANCH",
		"city_ref":       "city-ref-123",
		"city_name":      "Київ",
		"area_ref":       "area-ref-789",
		"area_name":      "Київська",
		"warehouse_ref":  "wh-ref-456",
		"warehouse_name": "Відділення №5",
		"is_default":     true,
	})
	req, _ := http.NewRequest(http.MethodPost, "/users/me/addresses", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var resp UserAddressResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "NOVA_POSHTA", resp.Provider)
	assert.Equal(t, "Київ", resp.CityName)
}

func TestUserHandler_CreateAddress_MissingFields(t *testing.T) {
	svc := &MockUserService{}
	userID := uuid.New()
	router := newUserRouter(svc, userID)

	// provider відсутній (обов'язкове поле)
	body := toJSON(t, map[string]string{
		"city_name": "Київ",
	})
	req, _ := http.NewRequest(http.MethodPost, "/users/me/addresses", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ==========================================
// UpdateAddress
// ==========================================

func TestUserHandler_UpdateAddress_Success(t *testing.T) {
	svc := &MockUserService{}
	userID := uuid.New()
	router := newUserRouter(svc, userID)

	addressID := uuid.New()
	updated := makeTestAddressHTTP(userID)
	updated.ID = addressID
	updated.CityName = "Харків"
	updated.FullAddress = "Київська область, м. Харків, Відділення №5"

	// Хендлер викликає GetAddresses для отримання existing адреси
	svc.On("GetAddresses", mock.Anything, userID).Return([]domain.UserAddress{*updated}, nil)
	svc.On("UpdateAddress", mock.Anything, userID, addressID, mock.Anything).Return(updated, nil)

	body := bytes.NewBufferString(`{"city_name": "Харків"}`)
	req, _ := http.NewRequest(http.MethodPatch, fmt.Sprintf("/users/me/addresses/%s", addressID), body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestUserHandler_UpdateAddress_InvalidUUID(t *testing.T) {
	svc := &MockUserService{}
	userID := uuid.New()
	router := newUserRouter(svc, userID)

	body := bytes.NewBufferString(`{"city_name": "Харків"}`)
	req, _ := http.NewRequest(http.MethodPatch, "/users/me/addresses/not-a-uuid", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ==========================================
// DeleteAddress
// ==========================================

func TestUserHandler_DeleteAddress_Success(t *testing.T) {
	svc := &MockUserService{}
	userID := uuid.New()
	router := newUserRouter(svc, userID)

	addressID := uuid.New()
	svc.On("DeleteAddress", mock.Anything, userID, addressID).Return(nil)

	req, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("/users/me/addresses/%s", addressID), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp MessageResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Contains(t, resp.Message, "deleted")
}

func TestUserHandler_DeleteAddress_InvalidUUID(t *testing.T) {
	svc := &MockUserService{}
	userID := uuid.New()
	router := newUserRouter(svc, userID)

	req, _ := http.NewRequest(http.MethodDelete, "/users/me/addresses/not-a-uuid", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUserHandler_DeleteAddress_NotFound(t *testing.T) {
	svc := &MockUserService{}
	userID := uuid.New()
	router := newUserRouter(svc, userID)

	addressID := uuid.New()
	svc.On("DeleteAddress", mock.Anything, userID, addressID).Return(domain.ErrUserNotFound)

	req, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("/users/me/addresses/%s", addressID), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ==========================================
// SaveCheckoutAddress
// ==========================================

func TestUserHandler_SaveCheckoutAddress_Success(t *testing.T) {
	svc := &MockUserService{}
	userID := uuid.New()
	r := gin.New()
	h := NewUserHandler(svc, &noopLogger{})
	r.POST("/checkout", func(c *gin.Context) {
		c.Set("user_id", userID)
		c.Next()
	}, h.SaveCheckoutAddress)

	created := makeTestAddressHTTP(userID)
	created.AddressName = "Адреса 2"
	created.IsDefault = false

	// Маємо одну існуючу адресу, щоб нова стала "Адреса 2"
	existingAddr := makeTestAddressHTTP(userID)
	existingAddr.WarehouseRef = "some-other-wh-ref"
	existing := []domain.UserAddress{*existingAddr}
	svc.On("GetAddresses", mock.Anything, userID).Return(existing, nil)
	svc.On("CreateAddress", mock.Anything, mock.MatchedBy(func(addr *domain.UserAddress) bool {
		return addr.AddressName == "Адреса 2" && addr.IsDefault == false
	})).Return(created, nil)

	body := toJSON(t, map[string]interface{}{
		"provider":       "NOVA_POSHTA",
		"delivery_type":  "BRANCH",
		"city_ref":       "city-ref-123",
		"city_name":      "Київ",
		"area_ref":       "area-ref-789",
		"area_name":      "Київська",
		"warehouse_ref":  "wh-ref-456",
		"warehouse_name": "Відділення №5",
	})
	req, _ := http.NewRequest(http.MethodPost, "/checkout", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	var resp UserAddressResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "Адреса 2", resp.AddressName)
}

func TestUserHandler_SaveCheckoutAddress_Duplicate(t *testing.T) {
	svc := &MockUserService{}
	userID := uuid.New()
	r := gin.New()
	h := NewUserHandler(svc, &noopLogger{})
	r.POST("/checkout", func(c *gin.Context) {
		c.Set("user_id", userID)
		c.Next()
	}, h.SaveCheckoutAddress)

	// Існуюча адреса з тими самими параметрами
	existingAddr := makeTestAddressHTTP(userID)
	existing := []domain.UserAddress{*existingAddr}
	svc.On("GetAddresses", mock.Anything, userID).Return(existing, nil)

	body := toJSON(t, map[string]interface{}{
		"provider":       existingAddr.Provider,
		"delivery_type":  existingAddr.DeliveryType,
		"city_ref":       existingAddr.CityRef,
		"city_name":      existingAddr.CityName,
		"area_ref":       existingAddr.AreaRef,
		"area_name":      existingAddr.AreaName,
		"warehouse_ref":  existingAddr.WarehouseRef,
		"warehouse_name": existingAddr.WarehouseName,
	})
	req, _ := http.NewRequest(http.MethodPost, "/checkout", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Очікуємо 200 OK і ту саму адресу
	assert.Equal(t, http.StatusOK, w.Code)
	var resp UserAddressResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, existingAddr.ID.String(), resp.ID)
}

func TestUserHandler_SaveCheckoutAddress_FirstAddress_IsDefault(t *testing.T) {
	svc := &MockUserService{}
	userID := uuid.New()
	r := gin.New()
	h := NewUserHandler(svc, &noopLogger{})
	r.POST("/checkout", func(c *gin.Context) {
		c.Set("user_id", userID)
		c.Next()
	}, h.SaveCheckoutAddress)

	created := makeTestAddressHTTP(userID)
	created.AddressName = "Адреса 1"
	created.IsDefault = true

	// Немає адрес -> нова має стати дефолтною
	svc.On("GetAddresses", mock.Anything, userID).Return([]domain.UserAddress{}, nil)
	svc.On("CreateAddress", mock.Anything, mock.MatchedBy(func(addr *domain.UserAddress) bool {
		return addr.AddressName == "Адреса 1" && addr.IsDefault == true
	})).Return(created, nil)

	body := toJSON(t, map[string]interface{}{
		"provider":       "NOVA_POSHTA",
		"delivery_type":  "BRANCH",
		"city_ref":       "city-ref-123",
		"city_name":      "Київ",
		"area_ref":       "area-ref-789",
		"area_name":      "Київська",
		"warehouse_ref":  "wh-ref-456",
		"warehouse_name": "Відділення №5",
	})
	req, _ := http.NewRequest(http.MethodPost, "/checkout", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
}
