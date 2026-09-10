package observability

import (
	"context"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
	eventsApplication "github.com/VladHrytsaiuk/ecommerce-core/internal/core/events/application"
	"go.opentelemetry.io/otel/trace"
)

// OutboxTracer is the OpenTelemetry implementation of the core Outbox
// worker's tracing port. It belongs to Platform so the application layer never
// imports OpenTelemetry or another infrastructure package.
type OutboxTracer struct{}

func NewOutboxTracer() OutboxTracer { return OutboxTracer{} }

func (OutboxTracer) ContinueDelivery(ctx context.Context, delivery events.Delivery) (context.Context, eventsApplication.Span) {
	ctx = ExtractOutboxTraceContext(ctx, OutboxTraceContext{
		TraceParent: delivery.TraceParent,
		TraceState:  delivery.TraceState,
		RequestID:   delivery.RequestID,
	})
	ctx, span := StartOutboxProcess(ctx, delivery.Topic, delivery.Consumer, delivery.EventID.String(), delivery.Attempts)
	return ctx, spanAdapter{span: span}
}

type spanAdapter struct{ span trace.Span }

func (adapter spanAdapter) End() { adapter.span.End() }

var _ eventsApplication.Tracer = OutboxTracer{}
