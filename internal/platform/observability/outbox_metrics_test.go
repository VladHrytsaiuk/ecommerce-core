package observability

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

// The counter only helps if it reaches the scrape. Defining it and registering
// it are two different things, and an unregistered metric collects silently
// forever. These read it back the way Prometheus does, through the default
// gatherer the /metrics handler serves.

func TestTheDeadDeliveryCounterReachesTheScrape(t *testing.T) {
	outboxDeliveries.Reset()
	NewOutboxMetrics().DeliveryOutcome("returns_settlement", "returns.settlement_requested.v1", "dead")

	got := gathered(t, map[string]string{
		"consumer": "returns_settlement",
		"topic":    "returns.settlement_requested.v1",
		"outcome":  "dead",
	})
	if got != 1 {
		t.Fatalf("scraped value = %v, want 1; a dead settlement must be visible to an alert", got)
	}
}

func TestOutcomesAreCountedSeparatelyPerConsumerAndTopic(t *testing.T) {
	// One consumer's healthy throughput must not hide another's dead queue.
	outboxDeliveries.Reset()
	metrics := NewOutboxMetrics()
	metrics.DeliveryOutcome("notifications", "orders.paid.v1", "completed")
	metrics.DeliveryOutcome("notifications", "orders.paid.v1", "completed")
	metrics.DeliveryOutcome("notifications", "orders.paid.v1", "dead")
	metrics.DeliveryOutcome("reports_projection", "orders.paid.v1", "dead")

	for _, want := range []struct {
		consumer, outcome string
		value             float64
	}{
		{"notifications", "completed", 2},
		{"notifications", "dead", 1},
		{"reports_projection", "dead", 1},
	} {
		got := gathered(t, map[string]string{"consumer": want.consumer, "topic": "orders.paid.v1", "outcome": want.outcome})
		if got != want.value {
			t.Fatalf("%s/%s = %v, want %v", want.consumer, want.outcome, got, want.value)
		}
	}
}

// gathered reads one labelled sample out of the registry the /metrics endpoint
// serves, returning -1 when the series is absent.
func gathered(t *testing.T, labels map[string]string) float64 {
	t.Helper()
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, family := range families {
		if family.GetName() != "outbox_deliveries_total" {
			continue
		}
		for _, metric := range family.GetMetric() {
			matched := 0
			for _, pair := range metric.GetLabel() {
				if labels[pair.GetName()] == pair.GetValue() {
					matched++
				}
			}
			if matched == len(labels) {
				return metric.GetCounter().GetValue()
			}
		}
	}
	return -1
}
