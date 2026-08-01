package errors

import (
	"errors"
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/stretchr/testify/assert"
)

type testStruct struct {
	Email    string `validate:"required,email"`
	Password string `validate:"required,min=8"`
	Name     string `validate:"required"`
}

func TestFormatValidationError_WithValidatorErrors(t *testing.T) {
	v := validator.New()

	// Пустий struct — всі поля required
	err := v.Struct(testStruct{})
	result := FormatValidationError(err)

	assert.Contains(t, result, "field 'Email' is required")
	assert.Contains(t, result, "field 'Password' is required")
	assert.Contains(t, result, "field 'Name' is required")
	assert.NotContains(t, result, "Key:")
}

func TestFormatValidationError_EmailValidation(t *testing.T) {
	v := validator.New()

	err := v.Struct(testStruct{
		Email:    "not-an-email",
		Password: "12345678",
		Name:     "Test",
	})
	result := FormatValidationError(err)

	assert.Contains(t, result, "field 'Email' must be a valid email address")
}

func TestFormatValidationError_MinLengthValidation(t *testing.T) {
	v := validator.New()

	err := v.Struct(testStruct{
		Email:    "test@example.com",
		Password: "short",
		Name:     "Test",
	})
	result := FormatValidationError(err)

	assert.Contains(t, result, "field 'Password' must be at least 8 characters")
}

func TestFormatValidationError_NonValidatorError(t *testing.T) {
	err := errors.New("some regular error")
	result := FormatValidationError(err)

	assert.Equal(t, "some regular error", result)
}

func TestFormatValidationError_MultipleErrors(t *testing.T) {
	v := validator.New()

	err := v.Struct(testStruct{
		Email:    "bad",
		Password: "123",
		Name:     "",
	})
	result := FormatValidationError(err)

	// Має бути через ";"
	assert.Contains(t, result, "; ")
}
