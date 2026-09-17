// Package domain contains provider-neutral transactional notification contracts.
package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

const (
	OrderPaidTemplate = "order_paid"
	// The remaining template keys the system can ask for. They lived as string
	// literals at their call sites, which is how eight of them came to exist
	// with no template behind any of them.
	BackInStockTemplate           = "back_in_stock"
	SupportAgentReplyTemplate     = "support_agent_reply"
	AbandonedCartTemplate         = "abandoned_cart"
	ReturnApprovedTemplate        = "return_approved"
	ReturnRejectedTemplate        = "return_rejected"
	ReturnReceivedTemplate        = "return_received"
	ReturnRefundedTemplate        = "return_refunded"
	SignInCodeTemplate            = "sign_in_code"
	EmailVerificationCodeTemplate = "email_verification_code"
	PasswordResetCodeTemplate     = "password_reset_code"
	MaxEmailOperationTimeout      = 30 * time.Second
)

type EmailMessage struct {
	MessageID string
	To        string
	Subject   string
	HTML      string
	Text      string
}

type DeliveryReceipt struct {
	ProviderMessageID string
}

// EmailSender is implemented by SMTP, SES, SendGrid or test adapters.
type EmailSender interface {
	Code() string
	Send(context.Context, EmailMessage) (DeliveryReceipt, error)
}

type OrderContact struct {
	OrderID uuid.UUID
	Email   string
	Locale  string
}

type Job struct {
	ID                uuid.UUID
	EventID           uuid.UUID
	OrderID           uuid.UUID
	DedupeKey         string
	Locale            string
	Provider          string
	PayloadCiphertext string
	Status            string
	Attempts          int
	LockedAt          *time.Time
	LockToken         *uuid.UUID
	CreatedAt         time.Time
}

// TemplateSynchronizer installs the shipped defaults for a locale. It never
// replaces a template that already exists, so a store's own wording survives
// every deployment.
type TemplateSynchronizer interface {
	SynchronizeTemplates(ctx context.Context, locale string, templates []Template) error
}

type Template struct {
	Key     string
	Locale  string
	Subject string
	HTML    string
	Text    string
}

type PayloadCipher interface {
	Encrypt([]byte) (string, error)
	Decrypt(string) ([]byte, error)
}

type TemplateRenderer interface {
	Render(context.Context, string, string, any) (EmailMessage, error)
}

type Repository interface {
	FindOrderContact(context.Context, uuid.UUID) (*OrderContact, error)
	FindTemplate(context.Context, string, string, string) (*Template, error)
	CreateOrderPaidJob(context.Context, Job) (*Job, bool, error)
	ClaimOrderPaidJob(context.Context, uuid.UUID, time.Time, time.Duration) (*Job, bool, error)
	RecordAttempt(context.Context, Job, string, DeliveryReceipt, error, time.Time, int) error
}
