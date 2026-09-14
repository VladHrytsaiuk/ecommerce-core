package domain

import (
	"context"
	"encoding/json"
	"errors"
	events "github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
	"github.com/google/uuid"
	"strings"
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
	// ErrErasurePolicyUnnamed refuses an executor that cannot say which policy it
	// applies. The erasure record is only an audit fact if it names the rules
	// the erasure followed.
	ErrErasurePolicyUnnamed = errors.New("the erasure executor does not name the policy it applies")
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
	// PolicyVersion names the erasure and retention policy this executor
	// applies, as the store's own records identify it. It is recorded with
	// every erasure, so a later audit can tell which rules a given erasure
	// followed. It is read before anything is erased: an executor that cannot
	// name its policy is refused before it has changed a single row.
	PolicyVersion() string
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

// TopicErasureCompleted records that an administrator approved an erasure and
// the deployment's ErasureExecutor carried it out, in the same transaction as
// both. It is an audit fact, not a work item: nothing consumes it.
//
// It carries no identifier of the person. It replaces
// privacy.erasure_requested.v1, which wrote the raw customer_id into an
// append-only table — so the one record of an erasure kept the very identifier
// the erasure removed. The name changed with it: the event is written after the
// erasure has happened, and "requested" described something it never was. The
// topic is renamed now rather than versioned in place because this core is
// copied to start stores, and every later moment is more expensive.
//
// The request id still reaches the person through privacy_requests.customer_id.
// Whether that link is kept as proof the request was honoured, or removed, is a
// question for the store's legal exception matrix rather than for this event;
// see docs/design/domain-events-erasure.md.
const TopicErasureCompleted = "privacy.erasure_completed.v1"

// ErasureOutcomeCompleted is the only outcome ever recorded. A failed erasure
// rolls its approval back, so no event is written for it.
const ErasureOutcomeCompleted = "completed"

// maxPolicyVersionLength bounds an operator-supplied label that lands in an
// append-only table forever.
const maxPolicyVersionLength = 64

// ErasureCompletedEvent is built only through NewErasureCompletedEvent. Its
// fields are unexported so no caller can attach a customer identifier or skip
// the policy check.
type ErasureCompletedEvent struct {
	requestID     uuid.UUID
	policyVersion string
	completedAt   time.Time
}

// NewErasureCompletedEvent validates the audit fact before anything is erased:
// the service builds it first, so an erasure that could not be recorded never
// happens.
func NewErasureCompletedEvent(requestID uuid.UUID, policyVersion string, completedAt time.Time) (ErasureCompletedEvent, error) {
	policyVersion = strings.TrimSpace(policyVersion)
	if policyVersion == "" {
		return ErasureCompletedEvent{}, ErrErasurePolicyUnnamed
	}
	if requestID == uuid.Nil || completedAt.IsZero() || len(policyVersion) > maxPolicyVersionLength {
		return ErasureCompletedEvent{}, ErrInvalid
	}
	return ErasureCompletedEvent{requestID: requestID, policyVersion: policyVersion, completedAt: completedAt.UTC()}, nil
}

func (ErasureCompletedEvent) Topic() string               { return TopicErasureCompleted }
func (ErasureCompletedEvent) AggregateType() string       { return "privacy_request" }
func (e ErasureCompletedEvent) AggregateID() uuid.UUID    { return e.requestID }
func (e ErasureCompletedEvent) IdempotencyKey() uuid.UUID { return e.requestID }
func (e ErasureCompletedEvent) OccurredAt() time.Time     { return e.completedAt }

// MarshalPayload goes through encoding/json. The old payload was assembled by
// string concatenation, which was only safe because a UUID cannot contain a
// quote; a policy label can.
func (e ErasureCompletedEvent) MarshalPayload() ([]byte, error) {
	return json.Marshal(struct {
		Version       int       `json:"version"`
		RequestID     uuid.UUID `json:"request_id"`
		Outcome       string    `json:"outcome"`
		PolicyVersion string    `json:"policy_version"`
		CompletedAt   time.Time `json:"completed_at"`
	}{Version: 1, RequestID: e.requestID, Outcome: ErasureOutcomeCompleted, PolicyVersion: e.policyVersion, CompletedAt: e.completedAt})
}

var _ events.DomainEvent = ErasureCompletedEvent{}
