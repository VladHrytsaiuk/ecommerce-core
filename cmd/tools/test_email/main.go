// Пакет main реалізує утиліту для тестування відправки листів (Verification та Password Reset).
package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/config"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/email"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
)

func main() {
	to := flag.String("to", "", "Email address to send test emails to")
	flag.Parse()

	if *to == "" {
		fmt.Println("Usage: go run cmd/tools/test_email/main.go -to=your-email@example.com")
		log.Fatal("Error: destination email (-to flag) is required")
	}

	// 1. Завантаження конфігурації
	cfg := config.Load()

	// 2. Ініціалізація логера
	logger.Init()
	l := logger.Log

	l.Infow("Starting email test...", "to", *to)

	// 3. Вибір та ініціалізація провайдера
	var provider email.Provider
	if cfg.SendGridAPIKey != "" {
		l.Info("Using SendGrid provider for testing")
		provider = email.NewSendGridProvider(cfg, l)
	} else if cfg.SMTPHost != "" && cfg.SMTPUser != "" {
		l.Info("Using SMTP provider for testing")
		provider = email.NewSMTPProvider(cfg, l)
	} else {
		l.Warn("No email credentials found in .env. Using ZapProvider (check console output)")
		provider = email.NewZapProvider(cfg.StoreLogoURL, l)
	}

	// 4. Тестовий лист верифікації
	verifyLink := fmt.Sprintf("%s/verify-email?code=TEST12&email=%s", cfg.FrontendURL, *to)
	l.Infof("Sending test verification email to %s...", *to)

	err := provider.SendVerificationEmail(*to, "TEST12", verifyLink)
	if err != nil {
		l.Fatalw("Failed to send verification email", "error", err)
	}

	// 5. Тестовий лист скидання пароля
	resetLink := fmt.Sprintf("%s/reset-password?code=SECRET99", cfg.FrontendURL)
	l.Infof("Sending test password reset email to %s...", *to)

	err = provider.SendPasswordResetEmail(*to, "SECRET99", resetLink)
	if err != nil {
		l.Fatalw("Failed to send password reset email", "error", err)
	}

	// 6. Тестовий лист нагадування про оплату
	paymentLink := fmt.Sprintf("%s/mock-payment?order_id=test-order", cfg.FrontendURL)
	cancelLink := fmt.Sprintf("%s/orders/test-order", cfg.FrontendURL)
	orderData := email.OrderEmailData{
		OrderNumber:  99999,
		CustomerName: "Test User",
		Email:        *to,
		Phone:        "+380991234567",
		TotalPrice:   150000,
		Items: []email.OrderItemEmail{
			{Name: "Test Product 1", Quantity: 1, Price: 100000, Total: 100000},
			{Name: "Test Product 2", Quantity: 2, Price: 25000, Total: 50000},
		},
		PaymentURL: paymentLink,
		CancelURL:  cancelLink,
	}

	l.Infof("Sending test payment reminder email to %s...", *to)
	err = provider.SendPaymentReminderEmail(*to, orderData)
	if err != nil {
		l.Fatalw("Failed to send payment reminder email", "error", err)
	}

	l.Info("Email test completed successfully!")
	fmt.Printf("\n✨ SUCCESS! Test emails (Verification, Password Reset, Payment Reminder) were sent to: %s\n", *to)
}
