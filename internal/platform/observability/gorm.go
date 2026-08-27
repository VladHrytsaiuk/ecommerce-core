package observability

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
)

// GORMPlugin creates database client spans without recording SQL text or bound
// values. Queries frequently contain PII, so operation-level telemetry is the
// only safe default for production tracing.
type GORMPlugin struct{}

func NewGORMPlugin() *GORMPlugin { return &GORMPlugin{} }

func (*GORMPlugin) Name() string { return "ecommerce-core-otel" }

func (p *GORMPlugin) Initialize(db *gorm.DB) error {
	if err := db.Callback().Create().Before("gorm:create").Register("otel:before_create", p.before("create")); err != nil {
		return err
	}
	if err := db.Callback().Create().After("gorm:create").Register("otel:after_create", p.after); err != nil {
		return err
	}
	if err := db.Callback().Query().Before("gorm:query").Register("otel:before_query", p.before("query")); err != nil {
		return err
	}
	if err := db.Callback().Query().After("gorm:query").Register("otel:after_query", p.after); err != nil {
		return err
	}
	if err := db.Callback().Update().Before("gorm:update").Register("otel:before_update", p.before("update")); err != nil {
		return err
	}
	if err := db.Callback().Update().After("gorm:update").Register("otel:after_update", p.after); err != nil {
		return err
	}
	if err := db.Callback().Delete().Before("gorm:delete").Register("otel:before_delete", p.before("delete")); err != nil {
		return err
	}
	if err := db.Callback().Delete().After("gorm:delete").Register("otel:after_delete", p.after); err != nil {
		return err
	}
	if err := db.Callback().Raw().Before("gorm:raw").Register("otel:before_raw", p.before("exec")); err != nil {
		return err
	}
	return db.Callback().Raw().After("gorm:raw").Register("otel:after_raw", p.after)
}

func (*GORMPlugin) before(operation string) func(*gorm.DB) {
	return func(db *gorm.DB) {
		if db == nil || db.Statement == nil {
			return
		}
		ctx := db.Statement.Context
		if ctx == nil {
			ctx = context.Background()
		}
		ctx, _ = otel.Tracer("ecommerce-core/postgres").Start(ctx, "postgresql "+operation,
			trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(
				attribute.String("db.system", "postgresql"),
				attribute.String("db.operation.name", operation),
			),
		)
		db.Statement.Context = ctx
	}
}

func (*GORMPlugin) after(db *gorm.DB) {
	if db == nil || db.Statement == nil {
		return
	}
	span := trace.SpanFromContext(db.Statement.Context)
	if db.Error != nil && db.Error != gorm.ErrRecordNotFound {
		span.RecordError(db.Error)
	}
	span.End()
}
