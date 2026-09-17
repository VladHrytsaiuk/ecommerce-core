// Package sanitize turns untrusted provider errors into small, PII-free codes
// suitable for persistence, metrics and structured logs.
package sanitize

import (
	"context"
	"errors"
	"strings"
)

// ErrorCode deliberately never returns provider text: SMTP responses can echo
// recipient addresses, message subjects, or other sensitive request content.
func ErrorCode(err error) string {
	if err == nil {
		return ""
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "invalid_encrypted_notification_payload"):
		return "notification_payload_invalid"
	case strings.Contains(message, "notification_job_in_progress"):
		return "notification_job_in_progress"
	case strings.Contains(message, "panic"):
		return "handler_panic"
	case strings.Contains(message, "auth"), strings.Contains(message, "credential"), strings.Contains(message, "password"):
		return "smtp_auth_failed"
	case strings.Contains(message, "timeout"), errors.Is(err, context.DeadlineExceeded):
		return "connection_timeout"
	case strings.Contains(message, "connection"), strings.Contains(message, "network"), strings.Contains(message, "refused"):
		return "connection_failed"
	case strings.Contains(message, "recipient"), strings.Contains(message, "email"), strings.Contains(message, "address"), strings.Contains(message, "@"):
		return "invalid_recipient"
	default:
		return "provider_delivery_failed"
	}
}
