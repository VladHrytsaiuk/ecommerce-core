package application

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	cart "github.com/VladHrytsaiuk/ecommerce-core/internal/abandoned_cart/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
)

const ConsumerCampaignProducer = "abandoned_cart_campaigns"

// CampaignProducer accepts only identity-only Cart/Checkout events and reads
// the current contact snapshot after delivery. This prevents PII in Outbox.
type CampaignProducer struct {
	repo     cart.Repository
	carts    cart.CartRecoveryReader
	contacts cart.CartRecoveryContactProvider
	delay    time.Duration
	now      func() time.Time
}

func NewCampaignProducer(repo cart.Repository, carts cart.CartRecoveryReader, contacts cart.CartRecoveryContactProvider, delay time.Duration) (*CampaignProducer, error) {
	if repo == nil || carts == nil || contacts == nil || delay <= 0 {
		return nil, fmt.Errorf("invalid abandoned-cart campaign producer")
	}
	return &CampaignProducer{repo: repo, carts: carts, contacts: contacts, delay: delay, now: func() time.Time { return time.Now().UTC() }}, nil
}

func (p *CampaignProducer) Topic() string { return events.TopicCartUpdated }
func (p *CampaignProducer) Handle(ctx context.Context, delivery events.Delivery) error {
	if delivery.Topic != events.TopicCartUpdated && delivery.Topic != events.TopicCheckoutEmailCaptured {
		return fmt.Errorf("unexpected abandoned-cart topic")
	}
	var payload struct {
		Version int       `json:"version"`
		CartID  uuid.UUID `json:"cart_id"`
	}
	if err := json.Unmarshal(delivery.Payload, &payload); err != nil || payload.Version != 1 || payload.CartID == uuid.Nil || payload.CartID != delivery.AggregateID {
		return fmt.Errorf("invalid abandoned-cart payload")
	}
	state, err := p.carts.GetCartState(ctx, payload.CartID)
	if err != nil {
		return err
	}
	if !state.IsActive || state.IsPaid || state.IsEmpty {
		return nil
	}
	contact, err := p.contacts.ContactForCart(ctx, payload.CartID)
	if err != nil {
		return err
	}
	if contact == nil || strings.TrimSpace(contact.Email) == "" {
		return nil
	}
	return p.repo.CreateOrReset(ctx, cart.Campaign{ID: uuid.New(), CartID: payload.CartID, CustomerID: contact.CustomerID, ContactEmail: strings.ToLower(strings.TrimSpace(contact.Email)), Step: 1, Status: "pending", DueAt: p.now().Add(p.delay)})
}

// TopicOnlyConsumer makes the two source topics independently dispatchable by
// the generic OutboxWorker without duplicating producer logic.
type TopicOnlyConsumer struct {
	topic    string
	producer *CampaignProducer
}

func (c TopicOnlyConsumer) Topic() string { return c.topic }
func (c TopicOnlyConsumer) Handle(ctx context.Context, delivery events.Delivery) error {
	return c.producer.Handle(ctx, delivery)
}

func NewTopicConsumer(topic string, producer *CampaignProducer) TopicOnlyConsumer {
	return TopicOnlyConsumer{topic: topic, producer: producer}
}
