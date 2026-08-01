package email

import (
	"fmt"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
)

// OrderEmailData — дані для формування листа про замовлення
type OrderEmailData struct {
	OrderNumber  int64
	CustomerName string
	Email        string
	Phone        string
	Items        []OrderItemEmail
	TotalPrice   int // в копійках
	Delivery     DeliveryEmail
	IsNewUser    bool   // чи це Silent Registration
	ManagerLink  string // secure link для менеджера (тільки для admin email)
	PaymentURL   string // посилання на оплату (для листів)
	CancelURL    string // посилання на скасування (для листів)
}

// ShipmentEmailData — дані для листа про створення ТТН
type ShipmentEmailData struct {
	OrderNumber    int64
	CustomerName   string
	Provider       string
	TrackingNumber string
	CityName       string
	WarehouseName  string
}

// OrderItemEmail — елемент замовлення для листа
type OrderItemEmail struct {
	Name     string
	SKU      string
	Quantity int
	Price    int // ціна за одиницю в копійках
	Total    int // quantity * price в копійках
}

// DeliveryEmail — інформація про доставку для листа
type DeliveryEmail struct {
	Provider      string
	CityName      string
	WarehouseName string
}

// Provider визначає контракт для відправки електронних листів.
type Provider interface {
	SendVerificationEmail(to, code, link string) error
	SendPasswordResetEmail(to, code, link string) error
	// Order-related emails
	SendOrderConfirmationEmail(to string, data OrderEmailData) error
	SendAdminOrderNotification(to string, data OrderEmailData) error
	SendPaymentReminderEmail(to string, data OrderEmailData) error
	SendSecurityWarningEmail(to string) error
	// Shipment-related emails
	SendShipmentCreatedEmail(to string, data ShipmentEmailData) error
	// Feedback-related emails
	SendFeedbackEmail(to string, feedbackType string, userEmail string, content string, mediaURL *string) error
}

// ZapProvider — це реалізація провайдера для розробки, яка просто логує вміст листа.
type ZapProvider struct {
	logoURL string
	logger  logger.Logger
}

func NewZapProvider(logoURL string, l logger.Logger) Provider {
	return &ZapProvider{
		logoURL: logoURL,
		logger:  l,
	}
}

func (p *ZapProvider) SendVerificationEmail(to, code, link string) error {
	p.logger.Infow("📧 Sending verification email (MOCK)",
		"to", to,
		"code", code,
		"link", link,
		"logo", p.logoURL,
	)

	// Також виводимо у форматі, який легко помітити в консолі
	fmt.Printf("\n--- [EMAIL TO: %s] ---\n", to)
	fmt.Printf("Subject: Verify your AquaWheel account\n")
	fmt.Printf("Code: %s\n", code)
	fmt.Printf("Link: %s\n", link)
	fmt.Printf("-------------------------------\n\n")

	return nil
}

func (p *ZapProvider) SendPasswordResetEmail(to, code, link string) error {
	p.logger.Infow("📧 Sending password reset email (MOCK)",
		"to", to,
		"code", code,
		"link", link,
		"logo", p.logoURL,
	)

	fmt.Printf("\n--- [PASSWORD RESET EMAIL TO: %s] ---\n", to)
	fmt.Printf("Subject: Reset your AquaWheel password\n")
	fmt.Printf("Code: %s\n", code)
	fmt.Printf("Link: %s\n", link)
	fmt.Printf("-------------------------------\n\n")

	return nil
}

func (p *ZapProvider) SendOrderConfirmationEmail(to string, data OrderEmailData) error {
	p.logger.Infow("📧 Sending order confirmation email (MOCK)",
		"to", to,
		"order_number", data.OrderNumber,
		"total_price", data.TotalPrice,
		"is_new_user", data.IsNewUser,
	)

	fmt.Printf("\n--- [ORDER CONFIRMATION EMAIL TO: %s] ---\n", to)
	fmt.Printf("Subject: Ваше замовлення №%d успішно оформлено!\n", data.OrderNumber)
	fmt.Printf("Customer: %s | Phone: %s\n", data.CustomerName, data.Phone)
	fmt.Printf("Total: %d копійок\n", data.TotalPrice)
	fmt.Printf("Items: %d\n", len(data.Items))
	fmt.Printf("Delivery: %s — %s, %s\n", data.Delivery.Provider, data.Delivery.CityName, data.Delivery.WarehouseName)
	if data.IsNewUser {
		fmt.Printf("⚡ New user created via silent registration\n")
	}
	fmt.Printf("-------------------------------\n\n")

	return nil
}

func (p *ZapProvider) SendAdminOrderNotification(to string, data OrderEmailData) error {
	p.logger.Infow("📧 Sending admin order notification (MOCK)",
		"to", to,
		"order_number", data.OrderNumber,
	)

	fmt.Printf("\n--- [ADMIN ORDER NOTIFICATION TO: %s] ---\n", to)
	fmt.Printf("Subject: Нове замовлення №%d\n", data.OrderNumber)
	fmt.Printf("Customer: %s | Email: %s | Phone: %s\n", data.CustomerName, data.Email, data.Phone)
	for _, item := range data.Items {
		fmt.Printf("  - %s (SKU: %s) x%d = %d коп.\n", item.Name, item.SKU, item.Quantity, item.Total)
	}
	fmt.Printf("Total: %d копійок\n", data.TotalPrice)
	fmt.Printf("-------------------------------\n\n")

	return nil
}

func (p *ZapProvider) SendPaymentReminderEmail(to string, data OrderEmailData) error {
	p.logger.Infow("📧 Sending payment reminder email (MOCK)",
		"to", to,
		"order_number", data.OrderNumber,
	)

	fmt.Printf("\n--- [PAYMENT REMINDER EMAIL TO: %s] ---\n", to)
	fmt.Printf("Subject: Нагадування про оплату замовлення №%d\n", data.OrderNumber)
	fmt.Printf("У вас залишилося 10 хвилин для оплати замовлення. Якщо ви не оплатите, замовлення буде автоматично скасовано.\n")
	fmt.Printf("Оплатити: %s\n", data.PaymentURL)
	fmt.Printf("Скасувати: %s\n", data.CancelURL)
	fmt.Printf("Total: %d копійок\n", data.TotalPrice)
	fmt.Printf("-------------------------------\n\n")

	return nil
}

func (p *ZapProvider) SendSecurityWarningEmail(to string) error {
	p.logger.Infow("📧 Sending security warning email (MOCK)",
		"to", to,
	)

	fmt.Printf("\n--- [SECURITY WARNING EMAIL TO: %s] ---\n", to)
	fmt.Printf("Subject: Замовлення з вашим email\n")
	fmt.Printf("Було створено замовлення з вашим email. Якщо це не ви — зверніться до підтримки.\n")
	fmt.Printf("-------------------------------\n\n")

	return nil
}

func (p *ZapProvider) SendShipmentCreatedEmail(to string, data ShipmentEmailData) error {
	p.logger.Infow("📧 Sending shipment created email (MOCK)",
		"to", to,
		"order_number", data.OrderNumber,
		"tracking_number", data.TrackingNumber,
		"provider", data.Provider,
	)

	fmt.Printf("\n--- [SHIPMENT CREATED EMAIL TO: %s] ---\n", to)
	fmt.Printf("Subject: Ваше замовлення №%d відправлено!\n", data.OrderNumber)
	fmt.Printf("Перевізник: %s\n", data.Provider)
	fmt.Printf("Номер ТТН: %s\n", data.TrackingNumber)
	fmt.Printf("Місто: %s | Відділення: %s\n", data.CityName, data.WarehouseName)
	fmt.Printf("-------------------------------\n\n")

	return nil
}

func (p *ZapProvider) SendFeedbackEmail(to string, feedbackType string, userEmail string, content string, mediaURL *string) error {
	p.logger.Infow("📧 Sending feedback email (MOCK)",
		"to", to,
		"type", feedbackType,
		"user_email", userEmail,
	)

	fmt.Printf("\n--- [FEEDBACK EMAIL TO: %s] ---\n", to)
	fmt.Printf("Subject: Нове звернення зворотнього зв'язку [%s]\n", feedbackType)
	fmt.Printf("User Email: %s\n", userEmail)
	fmt.Printf("Content: %s\n", content)
	if mediaURL != nil {
		fmt.Printf("Media Attachment: %s\n", *mediaURL)
	}
	fmt.Printf("-------------------------------\n\n")

	return nil
}

