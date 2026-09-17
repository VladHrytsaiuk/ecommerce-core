// Package notifications delivers Identity's one-time codes through the
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

// CodeMailer queues a one-time code email in the caller's transaction.
type CodeMailer struct {
	scheduler notifications.NotificationScheduler
}

func NewCodeMailer(scheduler notifications.NotificationScheduler) (*CodeMailer, error) {
	if scheduler == nil {
		return nil, fmt.Errorf("one-time code mailer requires the notification scheduler")
	}
	return &CodeMailer{scheduler: scheduler}, nil
}

func (*CodeMailer) Channel() identity.CodeChannel { return identity.CodeChannelEmail }

// templates names the email each purpose is sent with. Each says what the code
// is for, so a code nobody asked for tells its reader what someone tried.
var templates = map[identity.CodePurpose]string{
	identity.CodePurposeSignIn:        notifications.SignInCodeTemplate,
	identity.CodePurposeVerifyEmail:   notifications.EmailVerificationCodeTemplate,
	identity.CodePurposeResetPassword: notifications.PasswordResetCodeTemplate,
}

// codePayload is the template's data. CodeID is not shown; it keeps two
// messages carrying the same digits to the same address from being taken for
// one, which the queue's deduplication would otherwise drop.
type codePayload struct {
	CodeID           uuid.UUID `json:"CodeID"`
	Code             string    `json:"Code"`
	ExpiresInMinutes int       `json:"ExpiresInMinutes"`
}

func (m *CodeMailer) SendCode(ctx context.Context, message identity.CodeMessage) error {
	template, known := templates[message.Purpose]
	if m == nil || m.scheduler == nil || !known || message.ID == uuid.Nil || strings.TrimSpace(message.Destination) == "" || message.Code == "" || message.Lifetime <= 0 {
		return fmt.Errorf("invalid one-time code message")
	}
	return m.scheduler.ScheduleEmail(ctx, template, "", message.Destination, codePayload{
		CodeID:           message.ID,
		Code:             message.Code,
		ExpiresInMinutes: int(math.Ceil(message.Lifetime.Minutes())),
	})
}

var _ identity.CodeSender = (*CodeMailer)(nil)
