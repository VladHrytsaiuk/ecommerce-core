package observability

import (
	"github.com/prometheus/client_golang/prometheus"

	eventsApplication "github.com/VladHrytsaiuk/ecommerce-core/internal/core/events/application"
)

// outboxDeliveries counts how outbox deliveries end.
//
// The outcome that matters is "dead": a delivery that exhausted its retries is
// written to event_deliveries and nothing else happens. There is no admin
// surface over that table, so before this counter existed the only trace of a
// returns settlement that never ran — a refund a customer is still waiting for
// — was a row nobody queries. Alert on any increase in dead.
//
// Cardinality is bounded by the wiring: eight consumers, ten topics, three
// outcomes.
var outboxDeliveries = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "outbox_deliveries_total",
	Help: "Outbox deliveries by consumer, topic and final outcome.",
}, []string{"consumer", "topic", "outcome"})

func init() { prometheus.MustRegister(outboxDeliveries) }

// OutboxMetrics is the Prometheus implementation of the core Outbox worker's
// metrics port. It belongs to Platform so the application layer never imports
// a metrics library.
type OutboxMetrics struct{}

func NewOutboxMetrics() OutboxMetrics { return OutboxMetrics{} }

func (OutboxMetrics) DeliveryOutcome(consumer, topic, outcome string) {
	outboxDeliveries.WithLabelValues(consumer, topic, outcome).Inc()
}

var _ eventsApplication.Recorder = OutboxMetrics{}
