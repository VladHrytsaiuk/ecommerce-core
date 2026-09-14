package application

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	consent "github.com/VladHrytsaiuk/ecommerce-core/internal/consent/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
)

type privacyRepository struct {
	consent.Repository
	created   []consent.PrivacyRequest
	approved  *consent.PrivacyRequest
	request   consent.PrivacyRequest
	completed []uuid.UUID
}

func (r *privacyRepository) CreatePrivacyRequest(_ context.Context, request consent.PrivacyRequest) error {
	r.created = append(r.created, request)
	return nil
}

func (r *privacyRepository) ApprovePrivacyRequest(_ context.Context, _ uuid.UUID) (*consent.PrivacyRequest, error) {
	// "in_progress" is what the real repository writes; the fake used to say
	// "approved", which is not a status the schema allows.
	approved := r.request
	approved.Status = "in_progress"
	r.approved = &approved
	return &approved, nil
}

func (r *privacyRepository) CompletePrivacyRequest(_ context.Context, id uuid.UUID) error {
	r.completed = append(r.completed, id)
	return nil
}

// rollbackTransaction reports whether the unit of work was abandoned, which is
// what keeps a refused approval from leaving the request marked approved.
type rollbackTransaction struct{ rolledBack bool }

func (t *rollbackTransaction) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	if err := fn(ctx); err != nil {
		t.rolledBack = true
		return err
	}
	return nil
}

type countingPublisher struct {
	published int
	events    []events.DomainEvent
}

func (p *countingPublisher) Publish(_ context.Context, event events.DomainEvent) error {
	p.published++
	p.events = append(p.events, event)
	return nil
}

type recordingEraser struct {
	erased []uuid.UUID
	err    error
	// unnamedPolicy makes the executor unable to say which policy it applies.
	unnamedPolicy bool
}

const testErasurePolicy = "erasure-policy-2026-09"

func (e *recordingEraser) PolicyVersion() string {
	if e.unnamedPolicy {
		return "  "
	}
	return testErasurePolicy
}

func (e *recordingEraser) Erase(_ context.Context, customerID uuid.UUID) error {
	if e.err != nil {
		return e.err
	}
	e.erased = append(e.erased, customerID)
	return nil
}

func newPrivacyService(repository *privacyRepository, transaction *rollbackTransaction, publisher *countingPublisher) *Service {
	service := NewService(repository, nil)
	service.now = func() time.Time { return time.Unix(0, 0).UTC() }
	return service.WithAdminWorkflow(transaction, publisher)
}

func TestErasureRequestIsRefusedWhenNothingCanPerformIt(t *testing.T) {
	// The request used to be accepted, approved, and never carried out. A
	// customer told their data was deleted when it was not is worse off than
	// one told this store cannot delete it.
	repository := &privacyRepository{}
	service := newPrivacyService(repository, &rollbackTransaction{}, &countingPublisher{})

	err := service.PrivacyRequest(context.Background(), uuid.New(), "erasure")
	if !errors.Is(err, consent.ErrErasureUnsupported) {
		t.Fatalf("PrivacyRequest() error = %v, want ErrErasureUnsupported", err)
	}
	if len(repository.created) != 0 {
		t.Fatal("an erasure request was stored although nothing can carry it out")
	}
}

func TestAnExportIsRefusedAtIntakeWithoutAnExporter(t *testing.T) {
	// This test used to assert the opposite — that an export is accepted
	// whatever the deployment can do — and that was the defect: the request
	// was stored, approved, moved to in_progress and left there forever while
	// a statutory deadline ran. The right of access gets the same judgement as
	// the right to erasure.
	repository := &privacyRepository{}
	service := newPrivacyService(repository, &rollbackTransaction{}, &countingPublisher{})

	err := service.PrivacyRequest(context.Background(), uuid.New(), "export")

	if !errors.Is(err, consent.ErrExportUnsupported) {
		t.Fatalf("PrivacyRequest(export) error = %v, want ErrExportUnsupported", err)
	}
	if len(repository.created) != 0 {
		t.Fatal("an export request was queued that nothing can answer")
	}
}

func TestAnExportIsAcceptedWhereItCanBeProduced(t *testing.T) {
	repository := &privacyRepository{}
	service := newPrivacyService(repository, &rollbackTransaction{}, &countingPublisher{}).WithExport(&recordingExporter{})

	if err := service.PrivacyRequest(context.Background(), uuid.New(), "export"); err != nil {
		t.Fatalf("PrivacyRequest(export) error = %v", err)
	}
	if len(repository.created) != 1 || repository.created[0].RequestType != "export" {
		t.Fatalf("stored = %+v", repository.created)
	}
}

func TestApprovingAnExportProducesItAndClosesTheRequest(t *testing.T) {
	request := consent.PrivacyRequest{ID: uuid.New(), CustomerID: uuid.New(), RequestType: "export", Status: "pending"}
	repository := &privacyRepository{request: request}
	exporter := &recordingExporter{}
	service := newPrivacyService(repository, &rollbackTransaction{}, &countingPublisher{}).WithExport(exporter)

	approved, err := service.ApprovePrivacyRequest(context.Background(), request.ID)
	if err != nil {
		t.Fatalf("ApprovePrivacyRequest() error = %v", err)
	}
	if len(exporter.exported) != 1 || exporter.exported[0] != request.CustomerID {
		t.Fatalf("exported = %v, want the request's customer", exporter.exported)
	}
	if len(repository.completed) != 1 || approved.Status != "completed" {
		t.Fatalf("request left at %q with %d completions", approved.Status, len(repository.completed))
	}
}

func TestApprovingAnExportIsRefusedAndRolledBackWithoutAnExporter(t *testing.T) {
	// Requests stored before the intake guard existed are still in the table.
	// An administrator must not be able to close one as done.
	request := consent.PrivacyRequest{ID: uuid.New(), CustomerID: uuid.New(), RequestType: "export", Status: "pending"}
	repository := &privacyRepository{request: request}
	transaction := &rollbackTransaction{}
	service := newPrivacyService(repository, transaction, &countingPublisher{})

	_, err := service.ApprovePrivacyRequest(context.Background(), request.ID)

	if !errors.Is(err, consent.ErrExportUnsupported) {
		t.Fatalf("ApprovePrivacyRequest() error = %v, want ErrExportUnsupported", err)
	}
	if !transaction.rolledBack {
		t.Fatal("the approval committed; the request would read as handled")
	}
	if len(repository.completed) != 0 {
		t.Fatal("a request nothing answered was closed as completed")
	}
}

type recordingExporter struct{ exported []uuid.UUID }

func (e *recordingExporter) Export(_ context.Context, customerID uuid.UUID) error {
	e.exported = append(e.exported, customerID)
	return nil
}

func TestApprovingAnErasureIsRefusedAndRolledBackWithoutAnExecutor(t *testing.T) {
	// Requests submitted before the guard existed are still in the table. An
	// administrator must not be able to close one as done.
	repository := &privacyRepository{request: consent.PrivacyRequest{ID: uuid.New(), CustomerID: uuid.New(), RequestType: "erasure", Status: "pending"}}
	transaction := &rollbackTransaction{}
	publisher := &countingPublisher{}
	service := newPrivacyService(repository, transaction, publisher)

	approved, err := service.ApprovePrivacyRequest(context.Background(), repository.request.ID)
	if !errors.Is(err, consent.ErrErasureUnsupported) {
		t.Fatalf("ApprovePrivacyRequest() error = %v, want ErrErasureUnsupported", err)
	}
	if approved != nil {
		t.Fatalf("ApprovePrivacyRequest() returned %+v, want nothing approved", approved)
	}
	if !transaction.rolledBack {
		t.Fatal("the approval was committed; the request would show as approved with nothing erased")
	}
	if publisher.published != 0 {
		t.Fatal("an erasure event was published although nothing was erased")
	}
}

func TestApprovingAnErasureErasesInTheApprovalTransaction(t *testing.T) {
	repository := &privacyRepository{request: consent.PrivacyRequest{ID: uuid.New(), CustomerID: uuid.New(), RequestType: "erasure", Status: "pending"}}
	transaction := &rollbackTransaction{}
	publisher := &countingPublisher{}
	eraser := &recordingEraser{}
	service := newPrivacyService(repository, transaction, publisher).WithErasure(eraser)

	approved, err := service.ApprovePrivacyRequest(context.Background(), repository.request.ID)
	if err != nil {
		t.Fatalf("ApprovePrivacyRequest() error = %v", err)
	}
	if approved == nil || approved.Status != "completed" {
		t.Fatalf("ApprovePrivacyRequest() = %+v, want the request closed", approved)
	}
	if len(repository.completed) != 1 {
		t.Fatal("the erasure committed but the request stayed in progress")
	}
	if len(eraser.erased) != 1 || eraser.erased[0] != repository.request.CustomerID {
		t.Fatalf("erased = %v, want the request's customer", eraser.erased)
	}
	if publisher.published != 1 {
		t.Fatalf("published %d events, want the erasure recorded once", publisher.published)
	}
	if transaction.rolledBack {
		t.Fatal("a successful erasure was rolled back")
	}
}

func TestAFailedErasureLeavesTheRequestUnapproved(t *testing.T) {
	// Half-erased and marked done is the worst outcome available here.
	repository := &privacyRepository{request: consent.PrivacyRequest{ID: uuid.New(), CustomerID: uuid.New(), RequestType: "erasure", Status: "pending"}}
	transaction := &rollbackTransaction{}
	publisher := &countingPublisher{}
	failure := errors.New("customer store unavailable")
	service := newPrivacyService(repository, transaction, publisher).WithErasure(&recordingEraser{err: failure})

	if _, err := service.ApprovePrivacyRequest(context.Background(), repository.request.ID); !errors.Is(err, failure) {
		t.Fatalf("ApprovePrivacyRequest() error = %v, want the erasure failure", err)
	}
	if !transaction.rolledBack {
		t.Fatal("the approval survived a failed erasure")
	}
	if publisher.published != 0 {
		t.Fatal("an erasure event was published for an erasure that failed")
	}
}

func TestSupportsErasureDescribesTheDeployment(t *testing.T) {
	repository := &privacyRepository{}
	if newPrivacyService(repository, &rollbackTransaction{}, &countingPublisher{}).SupportsErasure() {
		t.Fatal("SupportsErasure() = true with no executor configured")
	}
	if !newPrivacyService(repository, &rollbackTransaction{}, &countingPublisher{}).WithErasure(&recordingEraser{}).SupportsErasure() {
		t.Fatal("SupportsErasure() = false with an executor configured")
	}
}

func TestAnErasureIsRecordedWithItsPolicyAndWithoutTheCustomer(t *testing.T) {
	// The one record of an erasure used to keep the raw customer_id the
	// erasure had just removed.
	request := consent.PrivacyRequest{ID: uuid.New(), CustomerID: uuid.New(), RequestType: "erasure", Status: "pending"}
	repository := &privacyRepository{request: request}
	publisher := &countingPublisher{}
	service := newPrivacyService(repository, &rollbackTransaction{}, publisher).WithErasure(&recordingEraser{})

	if _, err := service.ApprovePrivacyRequest(context.Background(), request.ID); err != nil {
		t.Fatalf("ApprovePrivacyRequest() error = %v", err)
	}
	if len(publisher.events) != 1 {
		t.Fatalf("published %d events, want one", len(publisher.events))
	}
	event := publisher.events[0]
	if event.Topic() != consent.TopicErasureCompleted {
		t.Fatalf("topic = %q, want %q", event.Topic(), consent.TopicErasureCompleted)
	}
	if event.AggregateID() != request.ID {
		t.Fatalf("aggregate = %s, want the privacy request %s", event.AggregateID(), request.ID)
	}
	raw, err := event.MarshalPayload()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), request.CustomerID.String()) {
		t.Fatalf("the erasure record carries the erased customer: %s", raw)
	}
	if !strings.Contains(string(raw), testErasurePolicy) {
		t.Fatalf("the erasure record does not name its policy: %s", raw)
	}
}

func TestAnExecutorThatCannotNameItsPolicyIsRefusedBeforeItErasesAnything(t *testing.T) {
	// The policy is read before Erase. Checking it afterwards would roll back
	// the database, but not whatever the executor had already done elsewhere.
	request := consent.PrivacyRequest{ID: uuid.New(), CustomerID: uuid.New(), RequestType: "erasure", Status: "pending"}
	repository := &privacyRepository{request: request}
	transaction := &rollbackTransaction{}
	publisher := &countingPublisher{}
	eraser := &recordingEraser{unnamedPolicy: true}
	service := newPrivacyService(repository, transaction, publisher).WithErasure(eraser)

	_, err := service.ApprovePrivacyRequest(context.Background(), request.ID)

	if !errors.Is(err, consent.ErrErasurePolicyUnnamed) {
		t.Fatalf("ApprovePrivacyRequest() error = %v, want ErrErasurePolicyUnnamed", err)
	}
	if len(eraser.erased) != 0 {
		t.Fatal("the executor erased data before its policy was checked")
	}
	if !transaction.rolledBack || publisher.published != 0 || len(repository.completed) != 0 {
		t.Fatalf("rolledBack=%v published=%d completed=%d, want a refused, unrecorded, open request",
			transaction.rolledBack, publisher.published, len(repository.completed))
	}
}

func TestAnUnnamedPolicyRefusesTheRequestAtIntake(t *testing.T) {
	// Accepting a request every approval would reject is the promise the
	// intake guard exists to avoid making.
	repository := &privacyRepository{}
	service := newPrivacyService(repository, &rollbackTransaction{}, &countingPublisher{}).WithErasure(&recordingEraser{unnamedPolicy: true})

	if err := service.PrivacyRequest(context.Background(), uuid.New(), "erasure"); !errors.Is(err, consent.ErrErasurePolicyUnnamed) {
		t.Fatalf("PrivacyRequest() error = %v, want ErrErasurePolicyUnnamed", err)
	}
	if len(repository.created) != 0 {
		t.Fatal("a request was stored that no approval could honour")
	}
	if service.SupportsErasure() {
		t.Fatal("SupportsErasure() = true for an executor that cannot name its policy")
	}
}
