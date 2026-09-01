package domain

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestReturnRequestTransitionsAppendImmutableHistory(t *testing.T) {
	now := time.Date(2026, time.September, 1, 12, 0, 0, 0, time.UTC)
	customerID := uuid.New()
	request, err := NewReturnRequest(uuid.New(), customerID, RefundModeFull, []ReturnItem{{
		VariantID: uuid.New(), Quantity: 1, Condition: ItemConditionUnopened,
	}}, Actor{Type: ActorTypeCustomer, ID: &customerID}, now)
	if err != nil {
		t.Fatalf("NewReturnRequest() error = %v", err)
	}
	if request.Status != ReturnStatusNew || len(request.History) != 1 || request.History[0].Status != ReturnStatusNew {
		t.Fatalf("initial request = %+v, want new status and one history record", request)
	}

	adminID := uuid.New()
	history, err := request.Approve(Actor{Type: ActorTypeAdmin, ID: &adminID}, "eligible", now.Add(time.Hour))
	if err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	if request.Status != ReturnStatusApproved || len(request.History) != 2 || history.Status != ReturnStatusApproved || history.ActorID == nil || *history.ActorID != adminID {
		t.Fatalf("approved request = %+v, history = %+v", request, history)
	}

	if _, err := request.Receive(Actor{Type: ActorTypeSystem}, "warehouse scan", now.Add(2*time.Hour)); err != nil {
		t.Fatalf("Receive() error = %v", err)
	}
	if _, err := request.MarkRefunded(Actor{Type: ActorTypeSystem}, "gateway refund confirmed", now.Add(3*time.Hour)); err != nil {
		t.Fatalf("MarkRefunded() error = %v", err)
	}
	if request.Status != ReturnStatusRefunded || len(request.History) != 4 {
		t.Fatalf("refunded request = %+v", request)
	}
}

func TestReturnRequestRejectsInvalidTransitionsAndActors(t *testing.T) {
	now := time.Now().UTC()
	customerID := uuid.New()
	request, err := NewReturnRequest(uuid.New(), customerID, RefundModePartial, []ReturnItem{{
		VariantID: uuid.New(), Quantity: 2, Condition: ItemConditionOpened,
	}}, Actor{Type: ActorTypeCustomer, ID: &customerID}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := request.Receive(Actor{Type: ActorTypeSystem}, "", now); !errors.Is(err, ErrInvalidReturnTransition) {
		t.Fatalf("Receive() error = %v, want invalid transition", err)
	}
	if _, err := request.Approve(Actor{Type: ActorTypeAdmin}, "", now); !errors.Is(err, ErrInvalidReturnActor) {
		t.Fatalf("Approve() error = %v, want invalid actor", err)
	}
}

func TestNewReturnRequestRejectsDuplicateItems(t *testing.T) {
	now := time.Now().UTC()
	customerID, variantID := uuid.New(), uuid.New()
	_, err := NewReturnRequest(uuid.New(), customerID, RefundModeFull, []ReturnItem{
		{VariantID: variantID, Quantity: 1, Condition: ItemConditionUnopened},
		{VariantID: variantID, Quantity: 1, Condition: ItemConditionDamaged},
	}, Actor{Type: ActorTypeCustomer, ID: &customerID}, now)
	if !errors.Is(err, ErrInvalidReturnItem) {
		t.Fatalf("NewReturnRequest() error = %v, want duplicate variant validation", err)
	}
}
