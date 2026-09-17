// Package mock is the development SMS sender: it writes texts to the log
// instead of sending them.
//
// That includes sign-in codes, which is the point — a developer reads the code
// from the log — and why startup refuses it where APP_ENV=production.
package mock

import (
	"context"
	"strings"
	"sync"
)

type Logger interface{ Infow(string, ...interface{}) }

// Message is a text the mock was asked to send.
type Message struct {
	Phone string
	Text  string
}

type Sender struct {
	logger Logger
	mu     sync.Mutex
	sent   []Message
}

func New(logger Logger) *Sender { return &Sender{logger: logger} }

func (s *Sender) Send(_ context.Context, phone, text string) error {
	s.mu.Lock()
	s.sent = append(s.sent, Message{Phone: phone, Text: text})
	s.mu.Unlock()
	if s.logger != nil {
		s.logger.Infow("mock SMS (not sent)", "recipient_masked", maskPhone(phone), "text", text)
	}
	return nil
}

// Sent returns what the mock was asked to send, for tests.
func (s *Sender) Sent() []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Message(nil), s.sent...)
}

// maskPhone keeps the country prefix and the last two digits.
func maskPhone(phone string) string {
	phone = strings.TrimSpace(phone)
	if len(phone) < 7 {
		return "[invalid]"
	}
	return phone[:4] + strings.Repeat("*", len(phone)-6) + phone[len(phone)-2:]
}
