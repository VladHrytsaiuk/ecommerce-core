package application

import (
	"context"
	"testing"

	"github.com/google/uuid"

	cartDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/cart/domain"
	checkout "github.com/VladHrytsaiuk/ecommerce-core/internal/checkout/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
)

type contactRepoFake struct{ saved checkout.ContactSnapshot }

func (f *contactRepoFake) UpsertContact(_ context.Context, value checkout.ContactSnapshot) error {
	f.saved = value
	return nil
}
func (*contactRepoFake) FindContact(context.Context, uuid.UUID) (*checkout.ContactSnapshot, error) {
	return nil, nil
}

type contactCartFake struct{ id uuid.UUID }

func (f contactCartFake) GetOrCreate(context.Context, cartDomain.Owner) (*cartDomain.Cart, error) {
	return &cartDomain.Cart{ID: f.id}, nil
}
func (contactCartFake) Add(context.Context, cartDomain.Owner, cartDomain.Item) (*cartDomain.Cart, error) {
	return nil, nil
}
func (contactCartFake) SetQuantity(context.Context, cartDomain.Owner, cartDomain.Item) (*cartDomain.Cart, error) {
	return nil, nil
}
func (contactCartFake) Remove(context.Context, cartDomain.Owner, uuid.UUID) (*cartDomain.Cart, error) {
	return nil, nil
}
func (contactCartFake) SetPromoCode(context.Context, cartDomain.Owner, string) (*cartDomain.Cart, error) {
	return nil, nil
}

type contactTxFake struct{ committed bool }

func (f *contactTxFake) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	err := fn(ctx)
	f.committed = err == nil
	return err
}

type contactPublisherFake struct{ payload []byte }

func (f *contactPublisherFake) Publish(_ context.Context, event events.DomainEvent) error {
	var err error
	f.payload, err = event.MarshalPayload()
	return err
}

type contactConsentFake struct{ called bool }

func (f *contactConsentFake) GrantActiveMarketing(context.Context, *uuid.UUID, string, string) error {
	f.called = true
	return nil
}

func TestContactCapturePersistsAndPublishesWithoutEmail(t *testing.T) {
	cartID, customerID := uuid.New(), uuid.New()
	repository, tx, publisher, consent := &contactRepoFake{}, &contactTxFake{}, &contactPublisherFake{}, &contactConsentFake{}
	service := NewContactCaptureService(repository, contactCartFake{id: cartID}, consent, tx, publisher)
	if err := service.CaptureContact(context.Background(), checkout.CaptureContactCommand{CartID: cartID, Owner: cartDomain.Owner{CustomerID: &customerID}, Email: "Buyer@example.test", MarketingOptIn: true, IPAddress: "203.0.113.5"}); err != nil {
		t.Fatal(err)
	}
	if !tx.committed || !consent.called {
		t.Fatal("expected one committed atomic workflow")
	}
	if repository.saved.Email != "buyer@example.test" {
		t.Fatalf("normalized email = %q", repository.saved.Email)
	}
	if string(publisher.payload) != `{"version":1,"cart_id":"`+cartID.String()+`"}` {
		t.Fatalf("PII leaked or invalid event payload: %s", publisher.payload)
	}
}

func TestContactCaptureRejectsForeignCart(t *testing.T) {
	service := NewContactCaptureService(&contactRepoFake{}, contactCartFake{id: uuid.New()}, nil, &contactTxFake{}, &contactPublisherFake{})
	sessionID := uuid.New()
	if err := service.CaptureContact(context.Background(), checkout.CaptureContactCommand{CartID: uuid.New(), Owner: cartDomain.Owner{SessionID: &sessionID}, Email: "buyer@example.test"}); err != checkout.ErrCartOwnership {
		t.Fatalf("error = %v", err)
	}
}
