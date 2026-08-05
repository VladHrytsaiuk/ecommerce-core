package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/inventory/domain"
	syncDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/sync/domain"
)

func TestImportServiceAppliesClaimedStockChange(t *testing.T) {
	states, stock := &fakeInboundStates{claim: true}, &fakeExternalStock{}
	service := NewImportService(states, stock, time.Minute)
	change := validChange()
	if err := service.ApplyStockChange(context.Background(), change); err != nil {
		t.Fatalf("ApplyStockChange() error = %v", err)
	}
	if stock.quantity != 7 || !states.applied || states.failed {
		t.Fatalf("import result quantity:%d applied:%t failed:%t", stock.quantity, states.applied, states.failed)
	}
}

func TestImportServiceSkipsDuplicateClaim(t *testing.T) {
	states, stock := &fakeInboundStates{}, &fakeExternalStock{}
	if err := NewImportService(states, stock, time.Minute).ApplyStockChange(context.Background(), validChange()); err != nil {
		t.Fatalf("ApplyStockChange() error = %v", err)
	}
	if stock.called || states.applied {
		t.Fatal("duplicate change must not mutate stock or state")
	}
}

func TestImportServiceRecordsFailureForRetry(t *testing.T) {
	states := &fakeInboundStates{claim: true}
	stock := &fakeExternalStock{err: errors.New("reserved stock exceeds ERP snapshot")}
	if err := NewImportService(states, stock, time.Minute).ApplyStockChange(context.Background(), validChange()); err == nil {
		t.Fatal("ApplyStockChange() error = nil")
	}
	if !states.failed || states.applied {
		t.Fatalf("failure state failed:%t applied:%t", states.failed, states.applied)
	}
}

func validChange() syncDomain.StockChange {
	return syncDomain.StockChange{Source: "1c", ExternalID: "stock-42", Version: "42", SourceUpdatedAt: time.Now().UTC(), PayloadHash: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", VariantID: uuid.New(), WarehouseID: uuid.New(), Quantity: 7}
}

type fakeInboundStates struct{ claim, applied, failed bool }

func (f *fakeInboundStates) ClaimStockChange(context.Context, syncDomain.StockChange, time.Time, time.Duration) (bool, error) {
	return f.claim, nil
}
func (f *fakeInboundStates) MarkStockChangeApplied(context.Context, syncDomain.StockChange) error {
	f.applied = true
	return nil
}
func (f *fakeInboundStates) MarkStockChangeFailed(context.Context, syncDomain.StockChange, error) error {
	f.failed = true
	return nil
}

type fakeExternalStock struct {
	quantity int
	called   bool
	err      error
}

func (f *fakeExternalStock) ReplaceExternalQuantity(_ context.Context, _, _ uuid.UUID, quantity int) error {
	f.called = true
	f.quantity = quantity
	return f.err
}

var _ domain.ExternalStockImporter = (*fakeExternalStock)(nil)
