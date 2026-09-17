// Package notifications delivers Identity's sign-in codes through the
// notifications module's durable email queue.
package notifications

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/google/uuid"

	identity "github.com/VladHrytsaiuk/ecommerce-core/internal/identity/domain"
	notifications "github.com/VladHrytsaiuk/ecommerce-core/internal/notifications/domain"
)

// SignInCodeMailer queues a sign-in code email in the caller's transaction.
type SignInCodeMailer struct {
	scheduler notifications.NotificationScheduler
}

func NewSignInCodeMailer(scheduler notifications.NotificationScheduler) (*SignInCodeMailer, error) {
	if scheduler == nil {
		return nil, fmt.Errorf("sign-in code mailer requires the notification scheduler")
	}
	return &SignInCodeMailer{scheduler: scheduler}, nil
}

func (*SignInCodeMailer) Channel() identity.SignInCodeChannel { return identity.SignInCodeEmail }

// signInCodePayload is the template's data. CodeID is not shown; it keeps two
// messages carrying the same digits to the same address from being taken for
// one, which the queue's deduplication would otherwise drop.
type signInCodePayload struct {
	CodeID           uuid.UUID `json:"CodeID"`
	Code             string    `json:"Code"`
	ExpiresInMinutes int       `json:"ExpiresInMinutes"`
}

func (m *SignInCodeMailer) SendSignInCode(ctx context.Context, message identity.SignInCodeMessage) error {
	if m == nil || m.scheduler == nil || message.ID == uuid.Nil || strings.TrimSpace(message.Destination) == "" || message.Code == "" || message.Lifetime <= 0 {
		return fmt.Errorf("invalid sign-in code message")
	}
	return m.scheduler.ScheduleEmail(ctx, notifications.SignInCodeTemplate, "", message.Destination, signInCodePayload{
		CodeID:           message.ID,
		Code:             message.Code,
		ExpiresInMinutes: int(math.Ceil(message.Lifetime.Minutes())),
	})
}

var _ identity.SignInCodeSender = (*SignInCodeMailer)(nil)
