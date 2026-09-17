// Package mock implements a development email adapter without network I/O.
package mock

import (
	"context"
	"strings"
	"sync"

	notificationsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/notifications/domain"
)

type Logger interface{ Infow(string, ...interface{}) }

type Sender struct {
	logger Logger
	mu     sync.Mutex
	sent   []notificationsDomain.EmailMessage
}

func New(logger Logger) *Sender { return &Sender{logger: logger} }

func (*Sender) Code() string { return "mock" }

func (s *Sender) Send(_ context.Context, message notificationsDomain.EmailMessage) (notificationsDomain.DeliveryReceipt, error) {
	s.mu.Lock()
	s.sent = append(s.sent, message)
	s.mu.Unlock()
	if s.logger != nil {
		s.logger.Infow("mock transactional email sent", "message_id", message.MessageID, "recipient_masked", maskEmail(message.To))
	}
	return notificationsDomain.DeliveryReceipt{ProviderMessageID: message.MessageID}, nil
}

func maskEmail(value string) string {
	address := strings.TrimSpace(value)
	local, domain, ok := strings.Cut(address, "@")
	if !ok || local == "" || domain == "" {
		return "[invalid]"
	}
	if len(local) == 1 {
		return local + "***@" + domain
	}
	return local[:1] + "***@" + domain
}

func (s *Sender) Sent() []notificationsDomain.EmailMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]notificationsDomain.EmailMessage(nil), s.sent...)
}

var _ notificationsDomain.EmailSender = (*Sender)(nil)
