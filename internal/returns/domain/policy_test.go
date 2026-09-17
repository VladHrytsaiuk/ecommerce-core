package domain

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestWindowEligibilityPolicyUsesDeliveryDateAndBoundary(t *testing.T) {
	now := time.Date(2026, time.September, 1, 12, 0, 0, 0, time.UTC)
	policy, err := NewWindowEligibilityPolicy(14)
	if err != nil {
		t.Fatal(err)
	}
	policy.now = func() time.Time { return now }
	request, order := eligibleRequestAndOrder(now.AddDate(0, 0, -14))

	if err := policy.IsEligible(order, request); err != nil {
		t.Fatalf("IsEligible() at exact cutoff error = %v", err)
	}
	policy.now = func() time.Time { return now.Add(time.Nanosecond) }
	if err := policy.IsEligible(order, request); !errors.Is(err, ErrReturnWindowExpired) {
		t.Fatalf("IsEligible() after cutoff error = %v, want expired", err)
	}
}

func TestWindowEligibilityPolicyRequiresDeliveredOrderAndSafeSnapshot(t *testing.T) {
	now := time.Now().UTC()
	policy, err := NewWindowEligibilityPolicy(14)
	if err != nil {
		t.Fatal(err)
	}
	policy.now = func() time.Time { return now }
	request, order := eligibleRequestAndOrder(now)
	order.Status = "processing"
	if err := policy.IsEligible(order, request); !errors.Is(err, ErrReturnOrderNotDelivered) {
		t.Fatalf("IsEligible() status error = %v, want not delivered", err)
	}
	order.Status = "delivered"
	order.CustomerID = nil
	if err := policy.IsEligible(order, request); !errors.Is(err, ErrReturnCustomerMismatch) {
		t.Fatalf("IsEligible() customer error = %v, want mismatch", err)
	}
}

func TestNewWindowEligibilityPolicyValidatesRange(t *testing.T) {
	for _, days := range []int{0, -1, 3651} {
		if _, err := NewWindowEligibilityPolicy(days); err == nil {
			t.Fatalf("NewWindowEligibilityPolicy(%d) error = nil", days)
		}
	}
}

func eligibleRequestAndOrder(deliveredAt time.Time) (ReturnRequest, OrderSnapshot) {
	orderID, customerID := uuid.New(), uuid.New()
	return ReturnRequest{OrderID: orderID, CustomerID: customerID}, OrderSnapshot{
		OrderID: orderID, CustomerID: &customerID, Status: "delivered", DeliveredAt: &deliveredAt,
	}
}
