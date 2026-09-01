package application

import (
	"context"
	"encoding/json"
	"fmt"
	availability "github.com/VladHrytsaiuk/ecommerce-core/internal/availability_notifications/domain"
	events "github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
	notifications "github.com/VladHrytsaiuk/ecommerce-core/internal/notifications/domain"
	"github.com/google/uuid"
	"net/mail"
	"strings"
	"time"
)

type Service struct {
	repo      availability.Repository
	customers availability.CustomerEmailReader
}

func NewService(r availability.Repository, readers ...availability.CustomerEmailReader) *Service {
	s := &Service{repo: r}
	if len(readers) > 0 {
		s.customers = readers[0]
	}
	return s
}
func (s *Service) Subscribe(ctx context.Context, variantID uuid.UUID, email string, customers ...uuid.UUID) (*availability.Subscription, bool, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" && len(customers) > 0 && customers[0] != uuid.Nil && s.customers != nil {
		var err error
		email, err = s.customers.EmailForCustomer(ctx, customers[0])
		if err != nil {
			return nil, false, err
		}
		email = strings.ToLower(strings.TrimSpace(email))
	}
	if variantID == uuid.Nil {
		return nil, false, availability.ErrInvalidSubscription
	}
	a, e := mail.ParseAddress(email)
	if e != nil || a.Address != email {
		return nil, false, availability.ErrInvalidSubscription
	}
	return s.repo.Create(ctx, availability.Subscription{VariantID: variantID, Email: email})
}

type Handler struct {
	repo      availability.Repository
	scheduler notifications.NotificationScheduler
	dbTx      func(context.Context, func(context.Context) error) error
}

func NewHandler(repo availability.Repository, n notifications.NotificationScheduler, tx func(context.Context, func(context.Context) error) error) *Handler {
	return &Handler{repo: repo, scheduler: n, dbTx: tx}
}
func (*Handler) Topic() string { return availability.TopicVariantAvailable }
func (h *Handler) Handle(ctx context.Context, d events.Delivery) error {
	var p struct {
		Version   int       `json:"version"`
		VariantID uuid.UUID `json:"variant_id"`
	}
	if err := json.Unmarshal(d.Payload, &p); err != nil || d.Topic != availability.TopicVariantAvailable || p.Version != 1 || p.VariantID == uuid.Nil || p.VariantID != d.AggregateID {
		return fmt.Errorf("invalid inventory availability event")
	}
	var subs []availability.Subscription
	err := h.dbTx(ctx, func(t context.Context) error {
		var e error
		subs, e = h.repo.MarkPendingNotified(t, p.VariantID, time.Now().UTC())
		if e != nil {
			return e
		}
		for _, s := range subs {
			payload := struct {
				SubscriptionID uuid.UUID `json:"subscription_id"`
				VariantID      uuid.UUID `json:"variant_id"`
			}{s.ID, s.VariantID}
			if e := h.scheduler.ScheduleEmail(t, notifications.BackInStockJobType, s.Email, payload); e != nil {
				return e
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

var _ interface {
	Topic() string
	Handle(context.Context, events.Delivery) error
} = (*Handler)(nil)
