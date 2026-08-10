// Package ses is the AWS SES adapter boundary. It is intentionally a safe
// placeholder until an AWS SDK client and credentials provider are configured
// for a deployment; selecting it never silently falls back to plaintext SMTP.
package ses

import (
	"context"
	"fmt"

	notificationsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/notifications/domain"
)

type Sender struct{}

func New() *Sender { return &Sender{} }

func (*Sender) Code() string { return "ses" }

func (*Sender) Send(context.Context, notificationsDomain.EmailMessage) (notificationsDomain.DeliveryReceipt, error) {
	return notificationsDomain.DeliveryReceipt{}, fmt.Errorf("AWS SES sender is not configured: install the SES SDK adapter and credentials provider")
}

var _ notificationsDomain.EmailSender = (*Sender)(nil)
