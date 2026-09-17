package projectors

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
	reports "github.com/VladHrytsaiuk/ecommerce-core/internal/reports/domain"
)

func TestRefundProjectorIsIdempotentAndReversesRevenue(t *testing.T) {
	t.Parallel()
	orderID, eventID, productID := uuid.New(), uuid.New(), uuid.New()
	refundedAt := time.Date(2026, time.January, 2, 22, 30, 0, 0, time.UTC)
	repository := &fakeRepository{}
	projector, err := NewRefundProjector(repository, fakeTransactions{}, fakeSnapshots{snapshot: reports.OrderAnalyticsSnapshot{
		OrderID: orderID, Currency: "UAH", TotalMinor: 1_250, Channel: "web",
		PurchasedProductItems: []reports.PurchasedProductItem{{ProductID: &productID, Quantity: 2, TotalMinor: 1_250, Currency: "UAH"}},
	}}, "Europe/Kyiv")
	if err != nil {
		t.Fatal(err)
	}
	amount, err := money.NewMoney(1_250, "UAH")
	if err != nil {
		t.Fatal(err)
	}
	event, err := events.NewOrderRefundedEvent(orderID, amount, refundedAt)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := event.MarshalPayload()
	if err != nil {
		t.Fatal(err)
	}
	delivery := events.Delivery{EventID: eventID, AggregateID: orderID, Topic: events.TopicOrderRefunded, Payload: payload}
	if err := projector.Handle(context.Background(), delivery); err != nil {
		t.Fatalf("first Handle() error = %v", err)
	}
	if err := projector.Handle(context.Background(), delivery); err != nil {
		t.Fatalf("duplicate Handle() error = %v", err)
	}
	if len(repository.sales) != 1 || repository.sales[0].RefundedOrdersCount != 1 || repository.sales[0].RefundRevenueMinor != 1_250 || repository.sales[0].NetRevenueMinor != -1_250 {
		t.Fatalf("sales = %+v", repository.sales)
	}
	if len(repository.products) != 1 || repository.products[0].NetRevenueMinor != -1_250 || repository.products[0].UnitsSold != 0 {
		t.Fatalf("products = %+v", repository.products)
	}
	if got := repository.sales[0].BucketDate.Format("2006-01-02"); got != "2026-01-03" {
		t.Fatalf("refund bucket date = %s, want 2026-01-03", got)
	}
}
