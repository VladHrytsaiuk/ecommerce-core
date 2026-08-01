package email

import (
	"fmt"

	"github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/config"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"go.uber.org/zap"
)

// SendGridProvider — продакшн-реалізація Provider через SendGrid Web API.
type SendGridProvider struct {
	apiKey  string
	from    *mail.Email
	logoURL string
	logger  logger.Logger
}

// NewSendGridProvider створює новий SendGrid-провайдер з конфігурації.
func NewSendGridProvider(cfg *config.Config, l logger.Logger) Provider {
	fromEmail, err := mail.ParseEmail(cfg.EmailFrom)
	if err != nil {
		l.Infow("⚠️ Failed to parse EMAIL_FROM, using raw value",
			"email_from", cfg.EmailFrom,
			"error", err,
		)
		fromEmail = mail.NewEmail("AquaWheel Store", cfg.EmailFrom)
	}

	l.Infow("📧 SendGrid email provider initialized",
		"from", cfg.EmailFrom,
	)

	return &SendGridProvider{
		apiKey:  cfg.SendGridAPIKey,
		from:    fromEmail,
		logoURL: cfg.StoreLogoURL,
		logger:  l,
	}
}

// SendVerificationEmail надсилає лист підтвердження email з гарним HTML-шаблоном.
func (p *SendGridProvider) SendVerificationEmail(to, code, link string) error {
	subject := "Підтвердіть вашу пошту — AquaWheel Store"
	htmlBody := buildVerificationEmailHTML(p.logoURL, code, link)

	return p.send(to, subject, htmlBody)
}

// SendPasswordResetEmail надсилає лист для скидання пароля з гарним HTML-шаблоном.
func (p *SendGridProvider) SendPasswordResetEmail(to, code, link string) error {
	subject := "Скидання пароля — AquaWheel Store"
	htmlBody := buildPasswordResetEmailHTML(p.logoURL, code, link)

	return p.send(to, subject, htmlBody)
}

// send — внутрішня функція для відправки листа через SendGrid Web API.
func (p *SendGridProvider) send(to, subject, htmlBody string) error {
	toEmail := mail.NewEmail("", to)
	message := mail.NewSingleEmail(p.from, subject, toEmail, "", htmlBody)

	client := sendgrid.NewSendClient(p.apiKey)
	response, err := client.Send(message)
	if err != nil {
		p.logger.Error("failed to send email via SendGrid",
			zap.String("to", to),
			zap.String("subject", subject),
			zap.Error(err),
		)
		return fmt.Errorf("failed to send email: %w", err)
	}

	// SendGrid повертає 2xx для успішних запитів
	if response.StatusCode >= 400 {
		p.logger.Error("SendGrid returned error status",
			zap.String("to", to),
			zap.Int("status_code", response.StatusCode),
			zap.String("body", response.Body),
		)
		return fmt.Errorf("SendGrid error: status %d, body: %s", response.StatusCode, response.Body)
	}

	p.logger.Infow("📧 Email sent successfully via SendGrid",
		"to", to,
		"subject", subject,
		"status_code", response.StatusCode,
	)

	return nil
}

// SendOrderConfirmationEmail надсилає лист-підтвердження замовлення клієнту.
func (p *SendGridProvider) SendOrderConfirmationEmail(to string, data OrderEmailData) error {
	subject := fmt.Sprintf("Ваше замовлення №%d успішно оформлено! — AquaWheel Store", data.OrderNumber)
	htmlBody := buildOrderConfirmationEmailHTML(p.logoURL, data)
	return p.send(to, subject, htmlBody)
}

// SendPaymentReminderEmail надсилає нагадування про оплату замовлення
func (p *SendGridProvider) SendPaymentReminderEmail(to string, data OrderEmailData) error {
	subject := fmt.Sprintf("Нагадування: оплатіть замовлення №%d", data.OrderNumber)

	var itemsHTML string
	for _, item := range data.Items {
		itemsHTML += fmt.Sprintf(`<li>%s x%d - %d грн</li>`, item.Name, item.Quantity, item.Total/100)
	}

	htmlContent := fmt.Sprintf(`
		<h2>Нагадування про оплату</h2>
		<p>У вас залишилося 10 хвилин для оплати замовлення №%d. Якщо замовлення не буде оплачено, воно автоматично скасується.</p>
		<p><strong>Сума до сплати:</strong> %d грн</p>
		<a href="%s" style="display:inline-block;padding:10px 20px;background-color:#007BFF;color:#ffffff;text-decoration:none;border-radius:5px;">Оплатити зараз</a>
		<br><br>
		<a href="%s" style="font-size:12px;color:#888888;text-decoration:underline;">Скасувати замовлення</a>
		<h3>Ваше замовлення:</h3>
		<ul>%s</ul>
	`, data.OrderNumber, data.TotalPrice/100, data.PaymentURL, data.CancelURL, itemsHTML)

	return p.send(to, subject, htmlContent)
}

// SendAdminOrderNotification надсилає сповіщення адміністратору про нове замовлення.
func (p *SendGridProvider) SendAdminOrderNotification(to string, data OrderEmailData) error {
	subject := fmt.Sprintf("🛒 Нове замовлення №%d — AquaWheel Store", data.OrderNumber)
	htmlBody := buildAdminOrderNotificationEmailHTML(p.logoURL, data)
	return p.send(to, subject, htmlBody)
}

// SendSecurityWarningEmail надсилає попередження про замовлення з цим email.
func (p *SendGridProvider) SendSecurityWarningEmail(to string) error {
	subject := "⚠️ Замовлення з вашим email — AquaWheel Store"
	htmlBody := buildSecurityWarningEmailHTML(p.logoURL)
	return p.send(to, subject, htmlBody)
}

// SendShipmentCreatedEmail надсилає клієнту сповіщення про створення ТТН.
func (p *SendGridProvider) SendShipmentCreatedEmail(to string, data ShipmentEmailData) error {
	subject := fmt.Sprintf("📦 Ваше замовлення №%d відправлено! — AquaWheel Store", data.OrderNumber)
	htmlBody := buildShipmentCreatedEmailHTML(p.logoURL, data)
	return p.send(to, subject, htmlBody)
}

// SendFeedbackEmail надсилає адміну сповіщення про новий відгук.
func (p *SendGridProvider) SendFeedbackEmail(to string, feedbackType string, userEmail string, content string, mediaURL *string) error {
	subject := fmt.Sprintf("🔔 Нове звернення зворотнього зв'язку — %s", feedbackType)
	htmlBody := buildFeedbackEmailHTML(p.logoURL, feedbackType, userEmail, content, mediaURL)
	return p.send(to, subject, htmlBody)
}
