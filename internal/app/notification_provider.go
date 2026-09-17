package app

import (
	"fmt"
	"strings"

	mockEmail "github.com/VladHrytsaiuk/ecommerce-core/internal/adapters/notification/email/mock"
	sesEmail "github.com/VladHrytsaiuk/ecommerce-core/internal/adapters/notification/email/ses"
	smtpEmail "github.com/VladHrytsaiuk/ecommerce-core/internal/adapters/notification/email/smtp"
	notificationsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/notifications/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/config"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
)

func newNotificationEmailSender(cfg *config.Config) (notificationsDomain.EmailSender, error) {
	if cfg == nil {
		return nil, fmt.Errorf("notification configuration is required")
	}
	switch strings.ToLower(strings.TrimSpace(cfg.NotificationEmailProvider)) {
	case "", "mock":
		return mockEmail.New(logger.Log), nil
	case "smtp":
		return smtpEmail.New(smtpEmail.Config{Host: cfg.SMTPHost, Port: cfg.SMTPPort, Username: cfg.SMTPUser, Password: cfg.SMTPPassword, From: cfg.SMTPFrom, TLSMode: cfg.SMTPTLSMode})
	case "ses":
		return sesEmail.New(), nil
	default:
		return nil, fmt.Errorf("unsupported NOTIFICATION_EMAIL_PROVIDER %q", cfg.NotificationEmailProvider)
	}
}
