package email

import (
	"fmt"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/config"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"go.uber.org/zap"
	"gopkg.in/gomail.v2"
)

// SMTPProvider — продакшн-реалізація Provider через SMTP (gomail).
type SMTPProvider struct {
	dialer  *gomail.Dialer
	from    string
	logoURL string
	logger  logger.Logger
}

// NewSMTPProvider створює новий SMTP-провайдер з конфігурації.
func NewSMTPProvider(cfg *config.Config, l logger.Logger) Provider {
	dialer := gomail.NewDialer(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUser, cfg.SMTPPassword)

	l.Infow("📧 SMTP email provider initialized",
		"host", cfg.SMTPHost,
		"port", cfg.SMTPPort,
		"user", cfg.SMTPUser,
		"from", cfg.SMTPFrom,
	)

	return &SMTPProvider{
		dialer:  dialer,
		from:    cfg.SMTPFrom,
		logoURL: cfg.StoreLogoURL,
		logger:  l,
	}
}

// SendVerificationEmail надсилає лист підтвердження email з гарним HTML-шаблоном.
func (p *SMTPProvider) SendVerificationEmail(to, code, link string) error {
	subject := "Підтвердіть вашу пошту — AquaWheel Store"
	htmlBody := buildVerificationEmailHTML(p.logoURL, code, link)

	return p.send(to, subject, htmlBody)
}

// SendPasswordResetEmail надсилає лист для скидання пароля з гарним HTML-шаблоном.
func (p *SMTPProvider) SendPasswordResetEmail(to, code, link string) error {
	subject := "Скидання пароля — AquaWheel Store"
	htmlBody := buildPasswordResetEmailHTML(p.logoURL, code, link)

	return p.send(to, subject, htmlBody)
}

// send — внутрішня функція для відправки листа через SMTP.
func (p *SMTPProvider) send(to, subject, htmlBody string) error {
	m := gomail.NewMessage()
	m.SetHeader("From", p.from)
	m.SetHeader("To", to)
	m.SetHeader("Subject", subject)
	m.SetBody("text/html", htmlBody)

	if err := p.dialer.DialAndSend(m); err != nil {
		p.logger.Error("failed to send email via SMTP",
			zap.String("to", to),
			zap.String("subject", subject),
			zap.Error(err),
		)
		return fmt.Errorf("failed to send email: %w", err)
	}

	p.logger.Infow("📧 Email sent successfully",
		"to", to,
		"subject", subject,
	)

	return nil
}

// SendOrderConfirmationEmail надсилає лист-підтвердження замовлення клієнту.
func (p *SMTPProvider) SendOrderConfirmationEmail(to string, data OrderEmailData) error {
	subject := fmt.Sprintf("Ваше замовлення №%d успішно оформлено! — AquaWheel Store", data.OrderNumber)
	htmlBody := buildOrderConfirmationEmailHTML(p.logoURL, data)
	return p.send(to, subject, htmlBody)
}

// SendPaymentReminderEmail надсилає нагадування про оплату замовлення
func (p *SMTPProvider) SendPaymentReminderEmail(to string, data OrderEmailData) error {
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
func (p *SMTPProvider) SendAdminOrderNotification(to string, data OrderEmailData) error {
	subject := fmt.Sprintf("🛒 Нове замовлення №%d — AquaWheel Store", data.OrderNumber)
	htmlBody := buildAdminOrderNotificationEmailHTML(p.logoURL, data)
	return p.send(to, subject, htmlBody)
}

// SendSecurityWarningEmail надсилає попередження про замовлення з цим email.
func (p *SMTPProvider) SendSecurityWarningEmail(to string) error {
	subject := "⚠️ Замовлення з вашим email — AquaWheel Store"
	htmlBody := buildSecurityWarningEmailHTML(p.logoURL)
	return p.send(to, subject, htmlBody)
}

// SendShipmentCreatedEmail надсилає клієнту сповіщення про створення ТТН.
func (p *SMTPProvider) SendShipmentCreatedEmail(to string, data ShipmentEmailData) error {
	subject := fmt.Sprintf("📦 Ваше замовлення №%d відправлено! — AquaWheel Store", data.OrderNumber)
	htmlBody := buildShipmentCreatedEmailHTML(p.logoURL, data)
	return p.send(to, subject, htmlBody)
}

// SendFeedbackEmail надсилає адміну сповіщення про новий відгук.
func (p *SMTPProvider) SendFeedbackEmail(to string, feedbackType string, userEmail string, content string, mediaURL *string) error {
	subject := fmt.Sprintf("🔔 Нове звернення зворотнього зв'язку — %s", feedbackType)
	htmlBody := buildFeedbackEmailHTML(p.logoURL, feedbackType, userEmail, content, mediaURL)
	return p.send(to, subject, htmlBody)
}
