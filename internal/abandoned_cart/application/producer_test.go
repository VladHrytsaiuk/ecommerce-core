package application

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	cart "github.com/VladHrytsaiuk/ecommerce-core/internal/abandoned_cart/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
)

type producerRepoFake struct{ campaign cart.Campaign }

func (*producerRepoFake) ClaimDue(context.Context, time.Time) (*cart.Campaign, error) {
	return nil, nil
}
func (*producerRepoFake) Update(context.Context, *cart.Campaign) error { return nil }
func (*producerRepoFake) Create(context.Context, cart.Campaign) error  { return nil }
func (f *producerRepoFake) CreateOrReset(_ context.Context, value cart.Campaign) error {
	f.campaign = value
	return nil
}
func (*producerRepoFake) Requeue(context.Context, uuid.UUID) error { return nil }

type producerCartFake struct{ cart.CartState }

func (f producerCartFake) GetCartState(context.Context, uuid.UUID) (cart.CartState, error) {
	return f.CartState, nil
}

type producerContactFake struct{ contact *cart.Contact }

func (f producerContactFake) ContactForCart(context.Context, uuid.UUID) (*cart.Contact, error) {
	return f.contact, nil
}

func TestCampaignProducerReadsContactInsteadOfOutboxPII(t *testing.T) {
	cartID := uuid.New()
	payload, err := events.NewCheckoutEmailCapturedEvent(cartID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	raw, err := payload.MarshalPayload()
	if err != nil {
		t.Fatal(err)
	}
	repo := &producerRepoFake{}
	producer, err := NewCampaignProducer(repo, producerCartFake{cart.CartState{IsActive: true}}, producerContactFake{contact: &cart.Contact{Email: "buyer@example.test"}}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := producer.Handle(context.Background(), events.Delivery{Topic: events.TopicCheckoutEmailCaptured, AggregateID: cartID, Payload: raw}); err != nil {
		t.Fatal(err)
	}
	if repo.campaign.CartID != cartID || repo.campaign.ContactEmail != "buyer@example.test" {
		t.Fatalf("unexpected campaign: %#v", repo.campaign)
	}
}
