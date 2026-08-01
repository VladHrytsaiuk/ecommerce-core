// Package sms надає абстракцію для відправки SMS-повідомлень
// (зокрема OTP-кодів верифікації телефону).
package sms

import (
	"context"
	"fmt"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
)

// Sender визначає контракт для відправки SMS.
type Sender interface {
	// SendVerificationCode відправляє OTP-код підтвердження на номер телефону.
	// phone очікується у будь-якому форматі (+380..., 0..., 380...) — реалізація сама нормалізує його.
	SendVerificationCode(ctx context.Context, phone, code string) error
}

// LogSender — реалізація для розробки, яка лише логує код у консоль (імітація SMS).
type LogSender struct {
	logger logger.Logger
}

// NewLogSender створює мок-відправника для локальної розробки.
func NewLogSender(l logger.Logger) Sender {
	return &LogSender{logger: l}
}

func (s *LogSender) SendVerificationCode(_ context.Context, phone, code string) error {
	s.logger.Infof("📱 [SMS MOCK] To: %s | Code: %s | AquaWheel Store verification", phone, code)

	fmt.Printf("\n--- [SMS TO: %s] ---\n", phone)
	fmt.Printf("Код підтвердження: %s\n", code)
	fmt.Printf("Дійсний протягом 2 хвилин.\n")
	fmt.Printf("-------------------------------\n\n")

	return nil
}
