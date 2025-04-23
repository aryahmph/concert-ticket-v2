package otel

import (
	"context"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type PgxCustomTracer struct{}

func (p PgxCustomTracer) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	ctx, span := Tracer.Start(ctx, "pgx.query", trace.WithSpanKind(trace.SpanKindClient))

	span.SetAttributes(
		attribute.String("db.system", "postgresql"),
		attribute.String("db.statement", data.SQL),
		attribute.Int("db.args.count", len(data.Args)),
	)

	return ctx
}

func (p PgxCustomTracer) TraceQueryEnd(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryEndData) {
	span := trace.SpanFromContext(ctx)
	defer span.End()

	if data.Err != nil {
		span.SetStatus(codes.Error, data.Err.Error())
		span.RecordError(data.Err)
	} else {
		span.SetStatus(codes.Ok, "")
		span.SetAttributes(
			attribute.Int64("db.rows_affected", data.CommandTag.RowsAffected()),
		)
	}
}
