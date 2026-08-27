package observability

import (
	"context"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
)

const outboxTracerName = "ecommerce-core/outbox"

// OutboxTraceContext is deliberately transport metadata, never business-event
// payload. Only W3C propagation fields and a correlation ID are persisted.
type OutboxTraceContext struct {
	TraceParent string
	TraceState  string
	RequestID   string
}

func CaptureOutboxTraceContext(ctx context.Context) OutboxTraceContext {
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	return OutboxTraceContext{
		TraceParent: boundedHeader(carrier.Get("traceparent"), 512),
		TraceState:  boundedHeader(carrier.Get("tracestate"), 512),
		RequestID:   boundedHeader(logger.RequestID(ctx), 128),
	}
}

func ExtractOutboxTraceContext(ctx context.Context, metadata OutboxTraceContext) context.Context {
	carrier := propagation.MapCarrier{}
	if metadata.TraceParent != "" {
		carrier.Set("traceparent", metadata.TraceParent)
	}
	if metadata.TraceState != "" {
		carrier.Set("tracestate", metadata.TraceState)
	}
	ctx = otel.GetTextMapPropagator().Extract(ctx, carrier)
	if metadata.RequestID != "" {
		ctx = logger.WithRequestID(ctx, metadata.RequestID)
	}
	return ctx
}

func StartOutboxPublish(ctx context.Context, topic string) (context.Context, trace.Span) {
	return otel.Tracer(outboxTracerName).Start(ctx, "outbox publish "+topic, trace.WithSpanKind(trace.SpanKindProducer), trace.WithAttributes(
		attribute.String("messaging.system", "postgresql_outbox"),
		attribute.String("messaging.destination.name", topic),
	))
}

func StartOutboxProcess(ctx context.Context, topic, consumer, eventID string, attempt int) (context.Context, trace.Span) {
	return otel.Tracer(outboxTracerName).Start(ctx, "outbox process "+topic, trace.WithSpanKind(trace.SpanKindConsumer), trace.WithAttributes(
		attribute.String("messaging.system", "postgresql_outbox"),
		attribute.String("messaging.destination.name", topic),
		attribute.String("messaging.consumer.name", consumer),
		attribute.String("messaging.message.id", eventID),
		attribute.Int("messaging.retry.count", attempt-1),
	))
}

func boundedHeader(value string, maximum int) string {
	value = strings.TrimSpace(value)
	if len(value) > maximum {
		return ""
	}
	return value
}
