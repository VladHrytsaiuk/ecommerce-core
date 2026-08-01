package errors

import (
	"errors"
	"fmt"
	"strings"

	"github.com/go-playground/validator/v10"
)

// FormatValidationError перетворює помилку від Gin ShouldBind / validator
// у зрозуміле повідомлення для клієнта.
// Якщо помилка не є validator.ValidationErrors, повертає err.Error() як є.
func FormatValidationError(err error) string {
	var ve validator.ValidationErrors
	if !errors.As(err, &ve) {
		return err.Error()
	}

	msgs := make([]string, 0, len(ve))
	for _, fe := range ve {
		msgs = append(msgs, formatFieldError(fe))
	}
	return strings.Join(msgs, "; ")
}

// formatFieldError генерує людський опис для одного поля.
func formatFieldError(fe validator.FieldError) string {
	field := fe.Field()

	switch fe.Tag() {
	case "required":
		return fmt.Sprintf("field '%s' is required", field)
	case "email":
		return fmt.Sprintf("field '%s' must be a valid email address", field)
	case "min":
		return fmt.Sprintf("field '%s' must be at least %s characters", field, fe.Param())
	case "max":
		return fmt.Sprintf("field '%s' must be at most %s characters", field, fe.Param())
	case "oneof":
		return fmt.Sprintf("field '%s' must be one of: %s", field, fe.Param())
	case "uuid":
		return fmt.Sprintf("field '%s' must be a valid UUID", field)
	case "url":
		return fmt.Sprintf("field '%s' must be a valid URL", field)
	case "gte":
		return fmt.Sprintf("field '%s' must be greater than or equal to %s", field, fe.Param())
	case "lte":
		return fmt.Sprintf("field '%s' must be less than or equal to %s", field, fe.Param())
	default:
		return fmt.Sprintf("field '%s' failed validation: %s", field, fe.Tag())
	}
}
