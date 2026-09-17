// Package observability owns cross-cutting telemetry setup and HTTP
// instrumentation. It deliberately has no dependency on any business module.
package observability

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
)

const tracerName = "ecommerce-core/http"

type Config struct {
	Enabled     bool
	Endpoint    string
	ServiceName string
	Environment string
}

var (
	requests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "http_server_requests_total",
		Help: "Total HTTP requests served by the public API.",
	}, []string{"route", "method", "status"})
	duration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "http_server_request_duration_seconds",
		Help: "HTTP request duration for the public API.",
	}, []string{"route", "method"})
)

func init() {
	prometheus.MustRegister(requests, duration)
}

// Init installs a process-wide TracerProvider. Export is opt-in so local and
// constrained environments retain trace context without requiring a collector.
func Init(ctx context.Context, cfg Config) (func(context.Context) error, error) {
	resource := resource.NewWithAttributes("",
		attribute.String("service.name", defaultString(cfg.ServiceName, "ecommerce-core")),
		attribute.String("deployment.environment.name", defaultString(cfg.Environment, "production")),
	)
	options := []sdktrace.TracerProviderOption{sdktrace.WithResource(resource)}
	if cfg.Enabled {
		endpoint := strings.TrimSpace(cfg.Endpoint)
		if endpoint == "" {
			return nil, fmt.Errorf("OTEL_EXPORTER_OTLP_ENDPOINT is required when OTel is enabled")
		}
		exporter, err := otlptracehttp.New(ctx,
			otlptracehttp.WithEndpoint(endpoint),
			otlptracehttp.WithInsecure(),
		)
		if err != nil {
			return nil, fmt.Errorf("create OTLP trace exporter: %w", err)
		}
		options = append(options, sdktrace.WithBatcher(exporter))
	} else {
		options = append(options, sdktrace.WithSampler(sdktrace.NeverSample()))
	}
	provider := sdktrace.NewTracerProvider(options...)
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	return provider.Shutdown, nil
}

// Middleware creates a server span and records bounded-cardinality RED metrics.
func Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := strings.TrimSpace(c.GetHeader("X-Request-ID"))
		if requestID == "" {
			requestID = uuid.NewString()
		}

		ctx := otel.GetTextMapPropagator().Extract(c.Request.Context(), propagation.HeaderCarrier(c.Request.Header))
		ctx = logger.WithRequestID(ctx, requestID)
		ctx, span := otel.Tracer(tracerName).Start(ctx, c.Request.Method+" "+c.Request.URL.Path)
		defer span.End()
		c.Request = c.Request.WithContext(ctx)
		c.Header("X-Request-ID", requestID)

		started := time.Now()
		c.Next()

		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		span.SetName(c.Request.Method + " " + route)
		status := c.Writer.Status()
		requests.WithLabelValues(route, c.Request.Method, fmt.Sprintf("%d", status)).Inc()
		duration.WithLabelValues(route, c.Request.Method).Observe(time.Since(started).Seconds())
		span.SetAttributes(
			attribute.String("http.route", route),
			attribute.Int("http.response.status_code", status),
		)
	}
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
