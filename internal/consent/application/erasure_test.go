package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	consent "github.com/VladHrytsaiuk/ecommerce-core/internal/consent/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
)

type privacyRepository struct {
	consent.Repository
	created  []consent.PrivacyRequest
	approved *consent.PrivacyRequest
	request  consent.PrivacyRequest
}

func (r *privacyRepository) CreatePrivacyRequest(_ context.Context, request consent.PrivacyRequest) error {
	r.created = append(r.created, request)
	return nil
}

func (r *privacyRepository) ApprovePrivacyRequest(_ context.Context, _ uuid.UUID) (*consent.PrivacyRequest, error) {
	approved := r.request
	approved.Status = "approved"
	r.approved = &approved
	return &approved, nil
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

type countingPublisher struct{ published int }

func (p *countingPublisher) Publish(context.Context, events.DomainEvent) error {
	p.published++
	return nil
}

type recordingEraser struct {
	erased []uuid.UUID
	err    error
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

func TestExportRequestIsUnaffectedByTheErasureGuard(t *testing.T) {
	repository := &privacyRepository{}
	service := newPrivacyService(repository, &rollbackTransaction{}, &countingPublisher{})

	if err := service.PrivacyRequest(context.Background(), uuid.New(), "export"); err != nil {
		t.Fatalf("PrivacyRequest(export) error = %v", err)
	}
	if len(repository.created) != 1 {
		t.Fatalf("stored %d requests, want the export to still be accepted", len(repository.created))
	}
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
	if approved == nil || approved.Status != "approved" {
		t.Fatalf("ApprovePrivacyRequest() = %+v", approved)
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
