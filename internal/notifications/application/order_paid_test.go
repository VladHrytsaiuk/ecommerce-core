package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
	notificationsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/notifications/domain"
)

func TestOrderPaidEventHandlerRecordsFailedSend(t *testing.T) {
	orderID, eventID := uuid.New(), uuid.New()
	repository := &fakeRepository{contact: &notificationsDomain.OrderContact{OrderID: orderID, Email: "buyer@example.com", Locale: "es"}, job: &notificationsDomain.Job{ID: uuid.New(), Status: "pending", PayloadCiphertext: `{"email":"buyer@example.com","order_number":"ES-100"}`}}
	handler := NewOrderPaidEventHandler(repository, passthroughCipher{}, fakeRenderer{}, fakeSender{err: errors.New("smtp unavailable")})
	event := events.Delivery{EventID: eventID, AggregateID: orderID, Topic: events.TopicOrderPaid, Payload: []byte(`{"version":1,"order_number":"ES-100"}`)}

	if err := handler.Handle(context.Background(), event); err == nil {
		t.Fatal("Handle() error = nil, want sender error")
	}
	if repository.attemptJobID != repository.job.ID || repository.attemptError == nil || repository.attemptProvider != "fake" {
		t.Fatalf("recorded attempt = job:%s provider:%s error:%v", repository.attemptJobID, repository.attemptProvider, repository.attemptError)
	}
}

func TestOrderPaidEventHandlerSkipsAlreadySentJob(t *testing.T) {
	orderID := uuid.New()
	repository := &fakeRepository{contact: &notificationsDomain.OrderContact{OrderID: orderID, Email: "buyer@example.com", Locale: "es"}, job: &notificationsDomain.Job{ID: uuid.New(), Status: "sent"}, created: false}
	handler := NewOrderPaidEventHandler(repository, passthroughCipher{}, fakeRenderer{}, fakeSender{})
	event := events.Delivery{EventID: uuid.New(), AggregateID: orderID, Topic: events.TopicOrderPaid, Payload: []byte(`{"version":1,"order_number":"ES-100"}`)}

	if err := handler.Handle(context.Background(), event); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if repository.attemptJobID != uuid.Nil {
		t.Fatalf("RecordAttempt() job = %s, want no retry", repository.attemptJobID)
	}
}

func TestOrderPaidEventHandlerDefersJobLeasedByAnotherWorker(t *testing.T) {
	orderID := uuid.New()
	repository := &fakeRepository{contact: &notificationsDomain.OrderContact{OrderID: orderID, Email: "buyer@example.com", Locale: "es"}, job: &notificationsDomain.Job{ID: uuid.New(), Status: "sending"}, claimOK: false}
	handler := NewOrderPaidEventHandler(repository, passthroughCipher{}, fakeRenderer{}, fakeSender{})
	event := events.Delivery{EventID: uuid.New(), AggregateID: orderID, Topic: events.TopicOrderPaid, Payload: []byte(`{"version":1,"order_number":"ES-100"}`)}
	if err := handler.Handle(context.Background(), event); !errors.Is(err, errNotificationJobInProgress) {
		t.Fatalf("Handle() error = %v, want leased-job retry", err)
	}
	if repository.attemptJobID != uuid.Nil {
		t.Fatalf("RecordAttempt() job = %s, want no second send", repository.attemptJobID)
	}
}

func TestOrderPaidEventHandlerKeepsSafeLeaseWhenPolicyIsTooShort(t *testing.T) {
	handler := NewOrderPaidEventHandler(&fakeRepository{}, passthroughCipher{}, fakeRenderer{}, fakeSender{})
	originalLease := handler.jobLease
	handler.WithDeliveryPolicy(notificationsDomain.MaxEmailOperationTimeout, 1)
	if handler.jobLease != originalLease {
		t.Fatalf("job lease = %s, want unsafe policy rejected", handler.jobLease)
	}
}

func TestOrderPaidEventHandlerRecordsAttemptWhenTemplateRenderingFails(t *testing.T) {
	orderID := uuid.New()
	repository := &fakeRepository{contact: &notificationsDomain.OrderContact{OrderID: orderID, Email: "buyer@example.com", Locale: "es"}, job: &notificationsDomain.Job{ID: uuid.New(), Status: "pending", PayloadCiphertext: `{"email":"buyer@example.com","order_number":"ES-100"}`}}
	handler := NewOrderPaidEventHandler(repository, passthroughCipher{}, failingRenderer{}, fakeSender{})
	event := events.Delivery{EventID: uuid.New(), AggregateID: orderID, Topic: events.TopicOrderPaid, Payload: []byte(`{"version":1,"order_number":"ES-100"}`)}
	if err := handler.Handle(context.Background(), event); err == nil {
		t.Fatal("Handle() error = nil, want renderer failure")
	}
	if repository.attemptJobID != repository.job.ID || repository.attemptError == nil {
		t.Fatalf("failed rendering was not recorded: job=%s error=%v", repository.attemptJobID, repository.attemptError)
	}
}

type fakeRepository struct {
	contact         *notificationsDomain.OrderContact
	job             *notificationsDomain.Job
	created         bool
	claimOK         bool
	attemptJobID    uuid.UUID
	attemptProvider string
	attemptError    error
}

func (r *fakeRepository) FindOrderContact(context.Context, uuid.UUID) (*notificationsDomain.OrderContact, error) {
	return r.contact, nil
}
func (*fakeRepository) FindTemplate(context.Context, string, string, string) (*notificationsDomain.Template, error) {
	return nil, nil
}
func (r *fakeRepository) CreateOrderPaidJob(context.Context, notificationsDomain.Job) (*notificationsDomain.Job, bool, error) {
	return r.job, r.created, nil
}
func (r *fakeRepository) ClaimOrderPaidJob(_ context.Context, _ uuid.UUID, _ time.Time, _ time.Duration) (*notificationsDomain.Job, bool, error) {
	if !r.claimOK && r.job.Status == "sending" {
		return nil, false, nil
	}
	token := uuid.New()
	r.job.LockToken = &token
	return r.job, true, nil
}
func (r *fakeRepository) RecordAttempt(_ context.Context, job notificationsDomain.Job, provider string, _ notificationsDomain.DeliveryReceipt, sendErr error, _ time.Time, _ int) error {
	r.attemptJobID, r.attemptProvider, r.attemptError = job.ID, provider, sendErr
	return nil
}

type fakeSender struct{ err error }

func (fakeSender) Code() string { return "fake" }
func (s fakeSender) Send(context.Context, notificationsDomain.EmailMessage) (notificationsDomain.DeliveryReceipt, error) {
	return notificationsDomain.DeliveryReceipt{}, s.err
}

type passthroughCipher struct{}

func (passthroughCipher) Encrypt(plaintext []byte) (string, error)  { return string(plaintext), nil }
func (passthroughCipher) Decrypt(ciphertext string) ([]byte, error) { return []byte(ciphertext), nil }

type fakeRenderer struct{}

func (fakeRenderer) Render(context.Context, string, string, any) (notificationsDomain.EmailMessage, error) {
	return notificationsDomain.EmailMessage{Subject: "receipt", HTML: "<p>receipt</p>", Text: "receipt"}, nil
}

type failingRenderer struct{}

func (failingRenderer) Render(context.Context, string, string, any) (notificationsDomain.EmailMessage, error) {
	return notificationsDomain.EmailMessage{}, errors.New("template database timeout")
}

var _ notificationsDomain.Repository = (*fakeRepository)(nil)
