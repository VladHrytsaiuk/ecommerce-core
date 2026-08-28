package application

import (
	"context"
	"errors"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/delivery/domain"
	"github.com/google/uuid"
	"testing"
	"time"
)

func TestDispatcherCompletesClaimedJobAfterCarrierCall(t *testing.T) {
	amount := mustMoney(100, "EUR")
	job := &domain.DispatchJob{ID: uuid.New(), OrderID: uuid.New(), Provider: "fake", IdempotencyKey: uuid.New(), DeclaredValue: amount}
	jobs := &fakeJobs{job: job}
	d := NewDispatcher(jobs, mustRegistry(t, dispatchCarrier{}), time.Minute)
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
	d := NewDispatcher(jobs, mustRegistry(t, dispatchCarrier{err: errors.New("down")}), time.Minute)
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
	d := NewDispatcher(jobs, mustRegistry(t, dispatchCarrier{err: errors.New("down")}), time.Minute)
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
}

func (f *fakeJobs) Claim(context.Context, time.Time) (*domain.DispatchJob, error) {
	j := f.job
	f.job = nil
	return j, nil
}
func (f *fakeJobs) Complete(_ context.Context, id uuid.UUID, _ domain.ShipmentResult) error {
	f.completed = id
	return nil
}
func (f *fakeJobs) Retry(_ context.Context, id uuid.UUID, _ error, _ time.Time) error {
	f.retried = id
	return nil
}
func (*fakeJobs) Fail(context.Context, uuid.UUID, error) error          { return nil }
func (f *fakeJobs) Dead(_ context.Context, id uuid.UUID, _ error) error { f.dead = id; return nil }

type dispatchCarrier struct{ err error }

func (dispatchCarrier) Code() string { return "fake" }
func (f dispatchCarrier) Quote(context.Context, domain.ShipmentQuoteRequest) ([]domain.ShippingOption, error) {
	return nil, nil
}
func (f dispatchCarrier) CreateShipment(context.Context, domain.CreateShipmentRequest) (domain.ShipmentResult, error) {
	return domain.ShipmentResult{TrackingNumber: "T"}, f.err
}
func (dispatchCarrier) Track(context.Context, domain.TrackingRequest) (domain.TrackingResult, error) {
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
