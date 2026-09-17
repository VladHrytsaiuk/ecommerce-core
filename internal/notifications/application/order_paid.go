// Package application turns durable domain events into transactional
// notification jobs. Provider I/O happens only after the outbox lease is held.
package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
	notificationsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/notifications/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/sanitize"
)

const (
	defaultJobLease      = 5 * time.Minute
	defaultMaxAttempts   = 10
	jobLeaseSafetyMargin = 5 * time.Second
)

var errNotificationJobInProgress = errors.New("notification_job_in_progress")

type OrderPaidEventHandler struct {
	repository  notificationsDomain.Repository
	cipher      notificationsDomain.PayloadCipher
	renderer    notificationsDomain.TemplateRenderer
	sender      notificationsDomain.EmailSender
	jobLease    time.Duration
	maxAttempts int
}

func NewOrderPaidEventHandler(repository notificationsDomain.Repository, cipher notificationsDomain.PayloadCipher, renderer notificationsDomain.TemplateRenderer, sender notificationsDomain.EmailSender) *OrderPaidEventHandler {
	return &OrderPaidEventHandler{repository: repository, cipher: cipher, renderer: renderer, sender: sender, jobLease: defaultJobLease, maxAttempts: defaultMaxAttempts}
}

func (h *OrderPaidEventHandler) WithDeliveryPolicy(jobLease time.Duration, maxAttempts int) *OrderPaidEventHandler {
	if jobLease > notificationsDomain.MaxEmailOperationTimeout+jobLeaseSafetyMargin {
		h.jobLease = jobLease
	}
	if maxAttempts > 0 {
		h.maxAttempts = maxAttempts
	}
	return h
}

func (*OrderPaidEventHandler) Topic() string { return events.TopicOrderPaid }

func (h *OrderPaidEventHandler) Handle(ctx context.Context, event events.Delivery) error {
	if h == nil || h.repository == nil || h.cipher == nil || h.renderer == nil || h.sender == nil {
		return fmt.Errorf("order paid notification handler is not configured")
	}
	if event.AggregateID == uuid.Nil {
		return fmt.Errorf("order paid event has no order id")
	}
	var payload struct {
		Version     int    `json:"version"`
		OrderNumber string `json:"order_number"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil || payload.Version != 1 || strings.TrimSpace(payload.OrderNumber) == "" {
		return fmt.Errorf("invalid order paid event payload")
	}
	contact, err := h.repository.FindOrderContact(ctx, event.AggregateID)
	if err != nil {
		return fmt.Errorf("find order contact: %w", err)
	}
	if contact == nil {
		return fmt.Errorf("order contact is missing")
	}
	jobPayload, err := json.Marshal(orderPaidJobPayload{Email: contact.Email, OrderNumber: payload.OrderNumber})
	if err != nil {
		return fmt.Errorf("marshal order paid notification payload: %w", err)
	}
	ciphertext, err := h.cipher.Encrypt(jobPayload)
	if err != nil {
		return fmt.Errorf("encrypt order paid notification payload: %w", err)
	}
	dedupeKey := "order_paid_" + event.AggregateID.String() + "_email"
	job, created, err := h.repository.CreateOrderPaidJob(ctx, notificationsDomain.Job{EventID: event.EventID, OrderID: event.AggregateID, DedupeKey: dedupeKey, Locale: contact.Locale, Provider: h.sender.Code(), PayloadCiphertext: ciphertext})
	if err != nil {
		return fmt.Errorf("create order paid notification job: %w", err)
	}
	if !created && (job.Status == "sent" || job.Status == "dead") {
		return nil
	}
	claimed, claimedOK, err := h.repository.ClaimOrderPaidJob(ctx, job.ID, time.Now().UTC(), h.jobLease)
	if err != nil {
		return fmt.Errorf("claim order paid notification job: %w", err)
	}
	if !claimedOK {
		// Do not acknowledge the outbox lease while another worker owns this
		// job. The delivery will retry after its backoff, and can reclaim a
		// genuinely abandoned notification lease later without a double send.
		return errNotificationJobInProgress
	}
	// The whole claimed delivery, including template I/O and SMTP, must finish
	// before the lease can be reclaimed. SMTP further caps itself at
	// MaxEmailOperationTimeout; this context also bounds a stalled renderer.
	deliveryCtx, cancel := context.WithTimeout(ctx, h.jobLease-jobLeaseSafetyMargin)
	defer cancel()
	plaintext, err := h.cipher.Decrypt(claimed.PayloadCiphertext)
	if err != nil {
		return h.recordClaimFailure(ctx, claimed, err)
	}
	var decrypted orderPaidJobPayload
	if err := json.Unmarshal(plaintext, &decrypted); err != nil || strings.TrimSpace(decrypted.Email) == "" || strings.TrimSpace(decrypted.OrderNumber) == "" {
		return h.recordClaimFailure(ctx, claimed, errors.New("invalid_encrypted_notification_payload"))
	}
	message, err := h.renderer.Render(deliveryCtx, notificationsDomain.OrderPaidTemplate, claimed.Locale, decrypted)
	if err != nil {
		return h.recordClaimFailure(ctx, claimed, err)
	}
	message.MessageID = claimed.ID.String()
	message.To = decrypted.Email
	receipt, sendErr := h.sender.Send(deliveryCtx, message)
	if sendErr != nil {
		return h.recordClaimFailure(ctx, claimed, sendErr)
	}
	if err := h.repository.RecordAttempt(context.WithoutCancel(ctx), *claimed, h.sender.Code(), receipt, nil, time.Now().UTC(), h.maxAttempts); err != nil {
		return fmt.Errorf("record order paid notification attempt: %w", err)
	}
	return nil
}

func (h *OrderPaidEventHandler) recordClaimFailure(ctx context.Context, claimed *notificationsDomain.Job, cause error) error {
	code := sanitize.ErrorCode(cause)
	if err := h.repository.RecordAttempt(context.WithoutCancel(ctx), *claimed, h.sender.Code(), notificationsDomain.DeliveryReceipt{}, errors.New(code), time.Now().UTC(), h.maxAttempts); err != nil {
		return fmt.Errorf("record order paid notification failure: %w", err)
	}
	return fmt.Errorf("order paid notification failed: %s", code)
}

// orderPaidJobPayload deliberately contains the only recipient PII needed to
// render and send the receipt. It is serialized exclusively into AES-GCM
// ciphertext and is never stored in notification_jobs as plaintext.
type orderPaidJobPayload struct {
	Email       string `json:"email"`
	OrderNumber string `json:"order_number"`
}

var _ interface {
	Topic() string
	Handle(context.Context, events.Delivery) error
} = (*OrderPaidEventHandler)(nil)
