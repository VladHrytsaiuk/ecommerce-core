package domain

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"time"
)

const (
	TopicVariantAvailable             = "inventory.variant.available.v1"
	ConsumerAvailabilityNotifications = "availability_notifications"
)

type Status string

const (
	StatusPending      Status = "pending"
	StatusNotified     Status = "notified"
	StatusUnsubscribed Status = "unsubscribed"
)

var ErrInvalidSubscription = errors.New("invalid stock subscription")

type Subscription struct {
	ID        uuid.UUID
	VariantID uuid.UUID
	Email     string
	Status    Status
	CreatedAt time.Time
}
type Repository interface {
	Create(context.Context, Subscription) (*Subscription, bool, error)
	MarkPendingNotified(context.Context, uuid.UUID, time.Time) ([]Subscription, error)
}

type CustomerEmailReader interface {
	EmailForCustomer(context.Context, uuid.UUID) (string, error)
}
