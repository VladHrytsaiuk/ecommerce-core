package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/user/domain"
)

func ptr[T any](v T) *T {
	return &v
}

func newUserSvc(
	userRepo *MockUserRepository,
	addressRepo *MockUserAddressRepository,
	sessionRepo *MockSessionRepository,
) domain.UserService {
	return NewUserService(userRepo, addressRepo, sessionRepo, &noopLogger{})
}

func makeTestAddress(userID uuid.UUID, isDefault bool) *domain.UserAddress {
	return &domain.UserAddress{
		ID:            uuid.New(),
		UserID:        userID,
		Provider:      "NOVA_POSHTA",
		DeliveryType:  "BRANCH",
		FullAddress:   "м. Київ, Відділення №5",
		CityRef:       "city-ref",
		CityName:      "Київ",
		WarehouseRef:  "wh-ref",
		WarehouseName: "Відділення №5",
		IsDefault:     isDefault,
	}
}

// ==========================================
// GetMe
// ==========================================

func TestUserService_GetMe_HappyPath(t *testing.T) {
	userRepo := &MockUserRepository{}
	svc := newUserSvc(userRepo, &MockUserAddressRepository{}, &MockSessionRepository{})

	userID := uuid.New()
	expected := &domain.User{ID: userID, Email: "test@example.com"}
	userRepo.On("FindByID", mock.Anything, userID).Return(expected, nil)

	result, err := svc.GetMe(context.Background(), userID)

	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestUserService_GetMe_UserNotFound(t *testing.T) {
	userRepo := &MockUserRepository{}
	svc := newUserSvc(userRepo, &MockUserAddressRepository{}, &MockSessionRepository{})

	userID := uuid.New()
	userRepo.On("FindByID", mock.Anything, userID).Return(nil, domain.ErrUserNotFound)

	result, err := svc.GetMe(context.Background(), userID)

	assert.Nil(t, result)
	assert.ErrorIs(t, err, domain.ErrUserNotFound)
}

// ==========================================
// UpdateMe
// ==========================================

func TestUserService_UpdateMe_HappyPath(t *testing.T) {
	userRepo := &MockUserRepository{}
	svc := newUserSvc(userRepo, &MockUserAddressRepository{}, &MockSessionRepository{})

	userID := uuid.New()
	existing := &domain.User{
		ID:        userID,
		FirstName: "Old",
		LastName:  "Name",
		Phone:     ptr("+380001112233"),
		Email:     "test@example.com",
	}
	input := &domain.UpdateMeInput{
		FirstName:       ptr("New"),
		LastName:        ptr("Name"),
		Phone:           ptr("+380999887766"),
		WantsNewsletter: ptr(true),
	}

	userRepo.On("FindByID", mock.Anything, userID).Return(existing, nil)
	userRepo.On("Update", mock.Anything, mock.Anything).Return(nil)

	result, err := svc.UpdateMe(context.Background(), userID, input)

	require.NoError(t, err)
	assert.Equal(t, "New", result.FirstName)
	assert.Equal(t, ptr("+380999887766"), result.Phone)
	assert.True(t, result.WantsNewsletter)
	// Email не повинен змінитися (захист від Mass Assignment)
	assert.Equal(t, "test@example.com", result.Email)
}

func TestUserService_UpdateMe_UserNotFound(t *testing.T) {
	userRepo := &MockUserRepository{}
	svc := newUserSvc(userRepo, &MockUserAddressRepository{}, &MockSessionRepository{})

	userID := uuid.New()
	userRepo.On("FindByID", mock.Anything, userID).Return(nil, domain.ErrUserNotFound)

	result, err := svc.UpdateMe(context.Background(), userID, &domain.UpdateMeInput{})

	assert.Nil(t, result)
	assert.ErrorIs(t, err, domain.ErrUserNotFound)
}

func TestUserService_UpdateMe_DoesNotChangeProtectedFields(t *testing.T) {
	userRepo := &MockUserRepository{}
	svc := newUserSvc(userRepo, &MockUserAddressRepository{}, &MockSessionRepository{})

	userID := uuid.New()
	existing := &domain.User{
		ID:              userID,
		Email:           "real@example.com",
		PasswordHash:    "secret-hash",
		IsEmailVerified: true,
		IsBlocked:       false,
		RoleID:          1,
	}
	// Зловмисний input намагається змінити захищені поля (навіть не маючи їх у структурі UpdateMeInput)
	input := &domain.UpdateMeInput{
		FirstName: ptr("Hacker"),
	}

	userRepo.On("FindByID", mock.Anything, userID).Return(existing, nil)
	userRepo.On("Update", mock.Anything, mock.MatchedBy(func(u *domain.User) bool {
		// Захищені поля не мають змінитися
		return u.IsEmailVerified == true && u.IsBlocked == false && u.RoleID == 1
	})).Return(nil)

	result, err := svc.UpdateMe(context.Background(), userID, input)

	require.NoError(t, err)
	assert.True(t, result.IsEmailVerified)
	assert.False(t, result.IsBlocked)
	assert.Equal(t, 1, result.RoleID)
	assert.Equal(t, "real@example.com", result.Email)
}

// ==========================================
// DeleteMe
// ==========================================

func TestUserService_DeleteMe_HappyPath(t *testing.T) {
	userRepo := &MockUserRepository{}
	sessionRepo := &MockSessionRepository{}
	svc := newUserSvc(userRepo, &MockUserAddressRepository{}, sessionRepo)

	userID := uuid.New()
	sessionRepo.On("DeleteAllForUser", mock.Anything, userID).Return(nil)
	userRepo.On("Delete", mock.Anything, userID).Return(nil)

	err := svc.DeleteMe(context.Background(), userID)

	require.NoError(t, err)
	sessionRepo.AssertExpectations(t)
	userRepo.AssertExpectations(t)
}

func TestUserService_DeleteMe_SessionDeletionFails_StillDeletesUser(t *testing.T) {
	// Якщо видалення сесій впало — продовжуємо видаляти юзера
	userRepo := &MockUserRepository{}
	sessionRepo := &MockSessionRepository{}
	svc := newUserSvc(userRepo, &MockUserAddressRepository{}, sessionRepo)

	userID := uuid.New()
	sessionRepo.On("DeleteAllForUser", mock.Anything, userID).Return(domain.ErrInternal)
	userRepo.On("Delete", mock.Anything, userID).Return(nil)

	err := svc.DeleteMe(context.Background(), userID)

	require.NoError(t, err)
}

// ==========================================
// GetAddresses
// ==========================================

func TestUserService_GetAddresses_HappyPath(t *testing.T) {
	addressRepo := &MockUserAddressRepository{}
	svc := newUserSvc(&MockUserRepository{}, addressRepo, &MockSessionRepository{})

	userID := uuid.New()
	addresses := []domain.UserAddress{
		*makeTestAddress(userID, true),
		*makeTestAddress(userID, false),
	}
	addressRepo.On("FindAllByUserID", mock.Anything, userID).Return(addresses, nil)

	result, err := svc.GetAddresses(context.Background(), userID)

	require.NoError(t, err)
	assert.Len(t, result, 2)
}

func TestUserService_GetAddresses_EmptyList(t *testing.T) {
	addressRepo := &MockUserAddressRepository{}
	svc := newUserSvc(&MockUserRepository{}, addressRepo, &MockSessionRepository{})

	userID := uuid.New()
	addressRepo.On("FindAllByUserID", mock.Anything, userID).Return([]domain.UserAddress{}, nil)

	result, err := svc.GetAddresses(context.Background(), userID)

	require.NoError(t, err)
	assert.Empty(t, result)
}

// ==========================================
// CreateAddress
// ==========================================

func TestUserService_CreateAddress_NonDefault(t *testing.T) {
	addressRepo := &MockUserAddressRepository{}
	svc := newUserSvc(&MockUserRepository{}, addressRepo, &MockSessionRepository{})

	userID := uuid.New()
	address := makeTestAddress(userID, false)

	addressRepo.On("Atomic", mock.Anything).Return(nil)
	addressRepo.On("Create", mock.Anything, address).Return(nil)

	result, err := svc.CreateAddress(context.Background(), address)

	require.NoError(t, err)
	assert.Equal(t, address, result)
}

func TestUserService_CreateAddress_Default_UnsetsOthers(t *testing.T) {
	addressRepo := &MockUserAddressRepository{}
	svc := newUserSvc(&MockUserRepository{}, addressRepo, &MockSessionRepository{})

	userID := uuid.New()
	address := makeTestAddress(userID, true) // IsDefault = true

	addressRepo.On("Atomic", mock.Anything).Return(nil)
	// Перед створенням дефолтної адреси — скидаємо всі інші
	addressRepo.On("UnsetDefaultAllForUser", mock.Anything, userID).Return(nil)
	addressRepo.On("Create", mock.Anything, address).Return(nil)

	result, err := svc.CreateAddress(context.Background(), address)

	require.NoError(t, err)
	assert.True(t, result.IsDefault)
	addressRepo.AssertCalled(t, "UnsetDefaultAllForUser", mock.Anything, userID)
}

// ==========================================
// UpdateAddress
// ==========================================

func TestUserService_UpdateAddress_HappyPath(t *testing.T) {
	addressRepo := &MockUserAddressRepository{}
	svc := newUserSvc(&MockUserRepository{}, addressRepo, &MockSessionRepository{})

	userID := uuid.New()
	addressID := uuid.New()
	existing := &domain.UserAddress{
		ID:        addressID,
		UserID:    userID,
		IsDefault: false,
	}
	update := &domain.UserAddress{
		FullAddress: "м. Харків, Відділення №10",
		IsDefault:   false,
	}

	addressRepo.On("FindByID", mock.Anything, addressID).Return(existing, nil)
	addressRepo.On("Atomic", mock.Anything).Return(nil)
	addressRepo.On("Update", mock.Anything, mock.Anything).Return(nil)

	result, err := svc.UpdateAddress(context.Background(), userID, addressID, update)

	require.NoError(t, err)
	assert.Equal(t, addressID, result.ID)
	assert.Equal(t, userID, result.UserID)
}

func TestUserService_UpdateAddress_AccessDenied_WrongUser(t *testing.T) {
	addressRepo := &MockUserAddressRepository{}
	svc := newUserSvc(&MockUserRepository{}, addressRepo, &MockSessionRepository{})

	ownerID := uuid.New()
	attackerID := uuid.New()
	addressID := uuid.New()

	existing := &domain.UserAddress{
		ID:     addressID,
		UserID: ownerID, // адреса належить іншому юзеру
	}
	addressRepo.On("FindByID", mock.Anything, addressID).Return(existing, nil)

	result, err := svc.UpdateAddress(context.Background(), attackerID, addressID, &domain.UserAddress{})

	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "access denied")
}

func TestUserService_UpdateAddress_AddressNotFound(t *testing.T) {
	addressRepo := &MockUserAddressRepository{}
	svc := newUserSvc(&MockUserRepository{}, addressRepo, &MockSessionRepository{})

	userID := uuid.New()
	addressID := uuid.New()
	addressRepo.On("FindByID", mock.Anything, addressID).Return(nil, domain.ErrUserNotFound)

	result, err := svc.UpdateAddress(context.Background(), userID, addressID, &domain.UserAddress{})

	assert.Nil(t, result)
	assert.ErrorIs(t, err, domain.ErrUserNotFound)
}

func TestUserService_UpdateAddress_SetDefault_UnsetsOthers(t *testing.T) {
	addressRepo := &MockUserAddressRepository{}
	svc := newUserSvc(&MockUserRepository{}, addressRepo, &MockSessionRepository{})

	userID := uuid.New()
	addressID := uuid.New()
	existing := &domain.UserAddress{
		ID:        addressID,
		UserID:    userID,
		IsDefault: false, // раніше НЕ дефолтна
	}
	update := &domain.UserAddress{
		IsDefault: true, // тепер робимо дефолтною
	}

	addressRepo.On("FindByID", mock.Anything, addressID).Return(existing, nil)
	addressRepo.On("Atomic", mock.Anything).Return(nil)
	addressRepo.On("UnsetDefaultAllForUser", mock.Anything, userID).Return(nil)
	addressRepo.On("Update", mock.Anything, mock.Anything).Return(nil)

	_, err := svc.UpdateAddress(context.Background(), userID, addressID, update)

	require.NoError(t, err)
	addressRepo.AssertCalled(t, "UnsetDefaultAllForUser", mock.Anything, userID)
}

// ==========================================
// DeleteAddress
// ==========================================

func TestUserService_DeleteAddress_HappyPath(t *testing.T) {
	addressRepo := &MockUserAddressRepository{}
	svc := newUserSvc(&MockUserRepository{}, addressRepo, &MockSessionRepository{})

	userID := uuid.New()
	addressID := uuid.New()
	existing := &domain.UserAddress{ID: addressID, UserID: userID}

	addressRepo.On("FindByID", mock.Anything, addressID).Return(existing, nil)
	addressRepo.On("Delete", mock.Anything, addressID).Return(nil)

	err := svc.DeleteAddress(context.Background(), userID, addressID)

	require.NoError(t, err)
	addressRepo.AssertExpectations(t)
}

func TestUserService_DeleteAddress_AccessDenied(t *testing.T) {
	addressRepo := &MockUserAddressRepository{}
	svc := newUserSvc(&MockUserRepository{}, addressRepo, &MockSessionRepository{})

	ownerID := uuid.New()
	attackerID := uuid.New()
	addressID := uuid.New()

	existing := &domain.UserAddress{ID: addressID, UserID: ownerID}
	addressRepo.On("FindByID", mock.Anything, addressID).Return(existing, nil)

	err := svc.DeleteAddress(context.Background(), attackerID, addressID)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "access denied")
	// Delete НЕ повинен бути викликаний
	addressRepo.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything)
}

func TestUserService_DeleteAddress_NotFound(t *testing.T) {
	addressRepo := &MockUserAddressRepository{}
	svc := newUserSvc(&MockUserRepository{}, addressRepo, &MockSessionRepository{})

	addressRepo.On("FindByID", mock.Anything, mock.Anything).
		Return(nil, fmt.Errorf("not found"))

	err := svc.DeleteAddress(context.Background(), uuid.New(), uuid.New())

	assert.Error(t, err)
}
