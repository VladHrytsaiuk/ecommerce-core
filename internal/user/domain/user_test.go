package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// ==========================================
// TableName Tests
// ==========================================

func TestUser_TableName(t *testing.T) {
	user := User{}
	// Ім'я таблиці обгорнуте в подвійні лапки для захисту від резервованого слова "user" в PostgreSQL
	assert.Equal(t, `"user"`, user.TableName())
}

func TestSession_TableName(t *testing.T) {
	session := Session{}
	assert.Equal(t, "session", session.TableName())
}

func TestUserAddress_TableName(t *testing.T) {
	address := UserAddress{}
	assert.Equal(t, "user_address", address.TableName())
}

// ==========================================
// Domain Errors Tests
// ==========================================

func TestDomainErrors_NotNil(t *testing.T) {
	errors := []error{
		ErrUserNotFound,
		ErrEmailAlreadyExists,
		ErrPhoneAlreadyExists,
		ErrSessionNotFound,
		ErrInvalidCredentials,
		ErrInternal,
	}

	for _, err := range errors {
		assert.NotNil(t, err, "Доменна помилка не повинна бути nil")
		assert.NotEmpty(t, err.Error(), "Повідомлення помилки не повинно бути пустим")
	}
}

func TestDomainErrors_UniqueMessages(t *testing.T) {
	errors := map[string]error{
		"ErrUserNotFound":       ErrUserNotFound,
		"ErrEmailAlreadyExists": ErrEmailAlreadyExists,
		"ErrPhoneAlreadyExists": ErrPhoneAlreadyExists,
		"ErrSessionNotFound":    ErrSessionNotFound,
		"ErrInvalidCredentials": ErrInvalidCredentials,
		"ErrInternal":           ErrInternal,
	}

	seen := make(map[string]string) // message -> error name
	for name, err := range errors {
		msg := err.Error()
		if prevName, exists := seen[msg]; exists {
			t.Errorf("Помилки %s і %s мають однакове повідомлення: %q", prevName, name, msg)
		}
		seen[msg] = name
	}
}

func TestDomainErrors_ExpectedMessages(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{"ErrUserNotFound", ErrUserNotFound, "user not found"},
		{"ErrEmailAlreadyExists", ErrEmailAlreadyExists, "email already exists"},
		{"ErrPhoneAlreadyExists", ErrPhoneAlreadyExists, "phone already exists"},
		{"ErrSessionNotFound", ErrSessionNotFound, "session not found"},
		{"ErrInvalidCredentials", ErrInvalidCredentials, "invalid email or password"},
		{"ErrInternal", ErrInternal, "internal server error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.err.Error())
		})
	}
}

// ==========================================
// User Entity Tests
// ==========================================

func TestUser_DefaultValues(t *testing.T) {
	user := User{}

	// Перевіряємо, що bool поля мають zero-value (false за замовчуванням)
	assert.False(t, user.IsEmailVerified)
	assert.False(t, user.IsPhoneVerified)
	assert.False(t, user.WantsNewsletter)
	assert.False(t, user.IsBlocked)
	assert.Nil(t, user.LastLoginAt, "LastLoginAt має бути nil для нового користувача")
}

func TestSession_DefaultValues(t *testing.T) {
	session := Session{}

	assert.False(t, session.IsBlocked)
	assert.Nil(t, session.UserAgent, "UserAgent має бути nil за замовчуванням")
	assert.Nil(t, session.ClientIP, "ClientIP має бути nil за замовчуванням")
}
