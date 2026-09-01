// Package notifications adapts the generic Notifications mail abstractions to
// the narrow availability port. It receives events only from an Outbox worker,
// never from an Inventory transaction.
package notifications

import (
	"context"
	"fmt"
	notifications "github.com/VladHrytsaiuk/ecommerce-core/internal/notifications/domain"
	"github.com/google/uuid"
)

type EmailPort struct {
	renderer notifications.TemplateRenderer
	sender   notifications.EmailSender
}

func NewEmailPort(r notifications.TemplateRenderer, s notifications.EmailSender) (*EmailPort, error) {
	if r == nil || s == nil {
		return nil, fmt.Errorf("availability notification dependencies are required")
	}
	return &EmailPort{r, s}, nil
}
func (p *EmailPort) EnqueueBackInStock(ctx context.Context, subscriptionID uuid.UUID, email string) error {
	message, err := p.renderer.Render(ctx, "back_in_stock", "en", struct{ VariantID string }{subscriptionID.String()})
	if err != nil {
		return err
	}
	message.MessageID = subscriptionID.String()
	message.To = email
	_, err = p.sender.Send(ctx, message)
	return err
}
