package domain

import (
	"context"
	"errors"
	events "github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
	"github.com/google/uuid"
	"time"
)

type LegalDocument struct {
	ID                        uuid.UUID
	Type, Version, ContentURL string
	PublishedAt               time.Time
	IsActive                  bool
}
type CustomerConsent struct {
	ID, CustomerID                uuid.UUID
	DocumentType, DocumentVersion string
	ContactEmail                  *string
	GrantedAt                     time.Time
	WithdrawnAt                   *time.Time
	IPAddress                     string
}
type PrivacyRequest struct {
	ID, CustomerID      uuid.UUID
	RequestType, Status string
	CreatedAt           time.Time
}

var (
	ErrInvalid                  = errors.New("invalid consent request")
	ErrDocumentInactive         = errors.New("legal document is not active")
	ErrTermsWithdrawalBlocked   = errors.New("terms consent cannot be withdrawn while orders are active")
	ErrInvalidPrivacyTransition = errors.New("invalid privacy request transition")
	// ErrErasureUnsupported is returned when a deployment has no ErasureExecutor.
	// Accepting the request anyway is worse than refusing it: the customer is
	// told their data will be deleted and it never is.
	ErrErasureUnsupported = errors.New("erasure is not available in this deployment")
	// ErrExportUnsupported is the same judgement for the right of access. An
	// export request used to be accepted, approved, moved to in_progress and
	// then left there: no exporter existed, so nothing was ever produced or
	// sent, and the clock on a statutory deadline ran while the request looked
	// like it was being handled.
	ErrExportUnsupported = errors.New("data export is not available in this deployment")
)

// ErasureExecutor deletes or anonymizes everything the core holds about one
// customer. The core deliberately ships no implementation.
//
// What must be erased, what must be retained, and for how long are not
// properties of this software — they follow from the store's jurisdiction,
// its tax and accounting obligations, and whatever it has told its customers.
// An order retained for a statutory period and a marketing profile deleted on
// request are both correct, and only the deployment knows which is which.
//
// Erase runs inside the approval transaction, so a failure leaves the request
// unapproved rather than half-erased.
type ErasureExecutor interface {
	Erase(ctx context.Context, customerID uuid.UUID) error
}

// DataExporter produces and delivers everything the core holds about one
// customer, for the right of access. Like ErasureExecutor the core ships no
// implementation, and for the same reason: what belongs in an export, what
// format it takes and how it reaches the customer are the store's decisions,
// not this software's.
//
// Export runs inside the approval transaction, so a failure leaves the request
// unapproved rather than half-delivered.
type DataExporter interface {
	Export(ctx context.Context, customerID uuid.UUID) error
}

type OrderActivityReader interface {
	HasActiveOrders(context.Context, uuid.UUID) (bool, error)
}
type Repository interface {
	ActiveDocuments(context.Context) ([]LegalDocument, error)
	Consents(context.Context, uuid.UUID) ([]CustomerConsent, error)
	Grant(context.Context, CustomerConsent) error
	Withdraw(context.Context, uuid.UUID, string, time.Time) error
	WithdrawMarketingByEmail(context.Context, string, time.Time) error
	CreatePrivacyRequest(context.Context, PrivacyRequest) error
	IsActiveDocument(context.Context, string, string) (bool, error)
	CreateDocument(context.Context, LegalDocument) error
	PublishDocument(context.Context, uuid.UUID) (*LegalDocument, error)
	ListPrivacyRequests(context.Context, int, int) ([]PrivacyRequest, int64, error)
	ApprovePrivacyRequest(context.Context, uuid.UUID) (*PrivacyRequest, error)
	CompletePrivacyRequest(context.Context, uuid.UUID) error
}
type TransactionManager interface {
	WithinTransaction(context.Context, func(context.Context) error) error
}

// TopicErasureRequested records an erasure that an administrator approved and
// the deployment's ErasureExecutor carried out, in the same transaction as
// both. It is an audit record of work done, not a work item: nothing consumes
// it, and nothing is waiting to. Approval is refused outright where no
// executor is configured, so this event never stands for a promise unkept.
const TopicErasureRequested = "privacy.erasure_requested.v1"

type ErasureRequestedEvent struct {
	EventID, CustomerID uuid.UUID
	At                  time.Time
}

func (ErasureRequestedEvent) Topic() string               { return TopicErasureRequested }
func (ErasureRequestedEvent) AggregateType() string       { return "privacy_request" }
func (e ErasureRequestedEvent) AggregateID() uuid.UUID    { return e.EventID }
func (e ErasureRequestedEvent) IdempotencyKey() uuid.UUID { return e.EventID }
func (e ErasureRequestedEvent) OccurredAt() time.Time     { return e.At }
func (e ErasureRequestedEvent) MarshalPayload() ([]byte, error) {
	return []byte(`{"version":1,"customer_id":"` + e.CustomerID.String() + `"}`), nil
}

var _ events.DomainEvent = ErasureRequestedEvent{}
