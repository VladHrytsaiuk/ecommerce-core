package application

import (
	"context"
	"errors"
	"fmt"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/delivery/domain"
	"github.com/google/uuid"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestDispatcherCompletesClaimedJobAfterCarrierCall(t *testing.T) {
	amount := mustMoney(100, "EUR")
	job := &domain.DispatchJob{ID: uuid.New(), OrderID: uuid.New(), Provider: "fake", IdempotencyKey: uuid.New(), DeclaredValue: amount}
	jobs := &fakeJobs{job: job}
	d := NewDispatcher(jobs, mustRegistry(t, &dispatchCarrier{}), time.Minute)
	if err := d.DispatchOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if jobs.completed != job.ID {
		t.Fatalf("completed=%v", jobs.completed)
	}
}
func TestDispatcherSchedulesRetryAfterCarrierError(t *testing.T) {
	amount := mustMoney(100, "EUR")
	job := &domain.DispatchJob{ID: uuid.New(), OrderID: uuid.New(), Provider: "fake", IdempotencyKey: uuid.New(), DeclaredValue: amount}
	jobs := &fakeJobs{job: job}
	d := NewDispatcher(jobs, mustRegistry(t, &dispatchCarrier{err: errors.New("down")}), time.Minute)
	if err := d.DispatchOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if jobs.retried != job.ID {
		t.Fatalf("retried=%v", jobs.retried)
	}
}

func TestDispatcherMovesExhaustedJobToDead(t *testing.T) {
	job := &domain.DispatchJob{ID: uuid.New(), OrderID: uuid.New(), Provider: "fake", IdempotencyKey: uuid.New(), DeclaredValue: mustMoney(100, "EUR"), Attempts: MaxAttempts}
	jobs := &fakeJobs{job: job}
	d := NewDispatcher(jobs, mustRegistry(t, &dispatchCarrier{err: errors.New("down")}), time.Minute)
	if err := d.DispatchOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if jobs.dead != job.ID || jobs.retried != uuid.Nil {
		t.Fatalf("dead=%v retried=%v", jobs.dead, jobs.retried)
	}
}

func TestDispatcherReconcilesExistingShipmentBeforeCreate(t *testing.T) {
	job := &domain.DispatchJob{ID: uuid.New(), OrderID: uuid.New(), Provider: "reconciling", IdempotencyKey: uuid.New(), DeclaredValue: mustMoney(100, "EUR")}
	jobs := &fakeJobs{job: job}
	carrier := &reconcilingCarrier{result: &domain.ShipmentResult{TrackingNumber: "already-created"}}
	registry, err := NewRegistry([]string{"reconciling"}, "reconciling", carrier)
	if err != nil {
		t.Fatal(err)
	}
	if err := NewDispatcher(jobs, registry, time.Minute).DispatchOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if jobs.completed != job.ID || carrier.createCalls != 0 {
		t.Fatalf("completed=%v createCalls=%d", jobs.completed, carrier.createCalls)
	}
}

type fakeJobs struct {
	job                      *domain.DispatchJob
	completed, retried, dead uuid.UUID
	retryDefinite            bool
	deadCause                error
}

func (f *fakeJobs) Claim(context.Context, time.Time) (*domain.DispatchJob, error) {
	j := f.job
	f.job = nil
	return j, nil
}
func (f *fakeJobs) Complete(_ context.Context, job domain.DispatchJob, _ domain.ShipmentResult) error {
	f.completed = job.ID
	return nil
}
func (f *fakeJobs) Retry(_ context.Context, job domain.DispatchJob, _ error, _ time.Time, definite bool) error {
	f.retried, f.retryDefinite = job.ID, definite
	return nil
}
func (*fakeJobs) Fail(context.Context, domain.DispatchJob, error) error { return nil }
func (f *fakeJobs) Dead(_ context.Context, job domain.DispatchJob, cause error) error {
	f.dead, f.deadCause = job.ID, cause
	return nil
}

// dispatchCarrier deliberately does not implement ShipmentFinder: it stands in
// for DHL Express, which cannot be asked whether a previous attempt worked.
type dispatchCarrier struct {
	err         error
	createCalls int
}

func (*dispatchCarrier) Code() string { return "fake" }
func (*dispatchCarrier) Quote(context.Context, domain.ShipmentQuoteRequest) ([]domain.ShippingOption, error) {
	return nil, nil
}
func (f *dispatchCarrier) CreateShipment(context.Context, domain.CreateShipmentRequest) (domain.ShipmentResult, error) {
	f.createCalls++
	return domain.ShipmentResult{TrackingNumber: "T"}, f.err
}
func (*dispatchCarrier) Track(context.Context, domain.TrackingRequest) (domain.TrackingResult, error) {
	return domain.TrackingResult{}, nil
}

type reconcilingCarrier struct {
	result      *domain.ShipmentResult
	createCalls int
}

func (*reconcilingCarrier) Code() string { return "reconciling" }
func (*reconcilingCarrier) Quote(context.Context, domain.ShipmentQuoteRequest) ([]domain.ShippingOption, error) {
	return nil, nil
}
func (c *reconcilingCarrier) CreateShipment(context.Context, domain.CreateShipmentRequest) (domain.ShipmentResult, error) {
	c.createCalls++
	return domain.ShipmentResult{}, nil
}
func (*reconcilingCarrier) Track(context.Context, domain.TrackingRequest) (domain.TrackingResult, error) {
	return domain.TrackingResult{}, nil
}
func (c *reconcilingCarrier) FindShipment(context.Context, string) (*domain.ShipmentResult, error) {
	return c.result, nil
}
func mustRegistry(t *testing.T, c domain.Carrier) *Registry {
	t.Helper()
	r, e := NewRegistry([]string{"fake"}, "fake", c)
	if e != nil {
		t.Fatal(e)
	}
	return r
}

func TestAmbiguousReattemptIsParkedWhenTheCarrierCannotBeAsked(t *testing.T) {
	// The duplicate waybill this guards against: the previous attempt timed
	// out, which is indistinguishable from a success whose response was lost,
	// and DHL has no lookup by the reference the adapter sends. Dispatching
	// again would print a second label for the same order.
	job := &domain.DispatchJob{ID: uuid.New(), OrderID: uuid.New(), Provider: "fake", IdempotencyKey: uuid.New(), DeclaredValue: mustMoney(100, "EUR"), Attempts: 2}
	jobs := &fakeJobs{job: job}
	carrier := &dispatchCarrier{}

	if err := NewDispatcher(jobs, mustRegistry(t, carrier), time.Minute).DispatchOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if carrier.createCalls != 0 {
		t.Fatalf("createCalls = %d; a second shipment was issued for an order that may already have one", carrier.createCalls)
	}
	if jobs.dead != job.ID {
		t.Fatalf("dead = %v, want the job parked for reconciliation", jobs.dead)
	}
	if jobs.deadCause == nil || !strings.Contains(jobs.deadCause.Error(), "reconcile manually") {
		t.Fatalf("dead cause = %v, want it to say what an operator must do", jobs.deadCause)
	}
}

func TestAReattemptAfterADefiniteFailureStillDispatches(t *testing.T) {
	// A request the carrier rejected outright created nothing, so there is no
	// duplicate to fear and parking the job would be needless manual work.
	job := &domain.DispatchJob{ID: uuid.New(), OrderID: uuid.New(), Provider: "fake", IdempotencyKey: uuid.New(), DeclaredValue: mustMoney(100, "EUR"), Attempts: 2, LastFailureWasDefinite: true}
	jobs := &fakeJobs{job: job}
	carrier := &dispatchCarrier{}

	if err := NewDispatcher(jobs, mustRegistry(t, carrier), time.Minute).DispatchOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if carrier.createCalls != 1 || jobs.completed != job.ID {
		t.Fatalf("createCalls = %d, completed = %v", carrier.createCalls, jobs.completed)
	}
}

func TestAReattemptDispatchesWhenTheCarrierCanBeAsked(t *testing.T) {
	// Nova Poshta can be queried, so an ambiguous previous attempt is resolved
	// by looking rather than by parking the job.
	job := &domain.DispatchJob{ID: uuid.New(), OrderID: uuid.New(), Provider: "reconciling", IdempotencyKey: uuid.New(), DeclaredValue: mustMoney(100, "EUR"), Attempts: 3}
	jobs := &fakeJobs{job: job}
	carrier := &reconcilingCarrier{}
	registry, err := NewRegistry([]string{"reconciling"}, "reconciling", carrier)
	if err != nil {
		t.Fatal(err)
	}

	if err := NewDispatcher(jobs, registry, time.Minute).DispatchOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if carrier.createCalls != 1 || jobs.dead != uuid.Nil {
		t.Fatalf("createCalls = %d, dead = %v", carrier.createCalls, jobs.dead)
	}
}

func TestTheDefinitenessOfAFailureIsRecordedForTheNextAttempt(t *testing.T) {
	for name, testCase := range map[string]struct {
		cause error
		want  bool
	}{
		"rejected by the carrier": {cause: fmt.Errorf("bad postcode: %w", domain.ErrShipmentNotSent), want: true},
		"timed out":               {cause: errors.New("context deadline exceeded"), want: false},
	} {
		t.Run(name, func(t *testing.T) {
			job := &domain.DispatchJob{ID: uuid.New(), OrderID: uuid.New(), Provider: "fake", IdempotencyKey: uuid.New(), DeclaredValue: mustMoney(100, "EUR"), Attempts: 1}
			jobs := &fakeJobs{job: job}

			if err := NewDispatcher(jobs, mustRegistry(t, &dispatchCarrier{err: testCase.cause}), time.Minute).DispatchOnce(context.Background()); err != nil {
				t.Fatal(err)
			}
			if jobs.retried != job.ID {
				t.Fatalf("retried = %v, want the job rescheduled", jobs.retried)
			}
			if jobs.retryDefinite != testCase.want {
				t.Fatalf("recorded definite = %t, want %t", jobs.retryDefinite, testCase.want)
			}
		})
	}
}

func TestRunDrainsTheBacklogInsteadOfOneJobPerTick(t *testing.T) {
	// On the default five-second tick, one job per pass was twelve dispatches
	// an hour: a backlog from any outage took days to clear.
	jobs := &queuedJobs{remaining: 7}
	ctx, cancel := context.WithCancel(context.Background())
	dispatcher := NewDispatcher(jobs, mustRegistry(t, &dispatchCarrier{}), time.Minute)

	done := make(chan struct{})
	go func() {
		dispatcher.Run(ctx, time.Hour)
		close(done)
	}()
	deadline := time.Now().Add(2 * time.Second)
	for jobs.left() > 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	cancel()
	<-done

	if left := jobs.left(); left != 0 {
		t.Fatalf("%d jobs left after one tick, want the backlog drained", left)
	}
}

type queuedJobs struct {
	mu        sync.Mutex
	remaining int
}

func (q *queuedJobs) left() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.remaining
}

func (q *queuedJobs) Claim(context.Context, time.Time) (*domain.DispatchJob, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.remaining == 0 {
		return nil, nil
	}
	q.remaining--
	return &domain.DispatchJob{ID: uuid.New(), OrderID: uuid.New(), Provider: "fake", IdempotencyKey: uuid.New(), DeclaredValue: mustMoney(100, "EUR"), Attempts: 1}, nil
}
func (*queuedJobs) Complete(context.Context, domain.DispatchJob, domain.ShipmentResult) error {
	return nil
}
func (*queuedJobs) Retry(context.Context, domain.DispatchJob, error, time.Time, bool) error {
	return nil
}
func (*queuedJobs) Fail(context.Context, domain.DispatchJob, error) error { return nil }
func (*queuedJobs) Dead(context.Context, domain.DispatchJob, error) error { return nil }
