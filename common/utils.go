package common

import (
	"concert-ticket/common/constant"
	"concert-ticket/common/otel"
	"context"
	"encoding/json"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/oklog/ulid/v2"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"log/slog"
)

func ExtractTraceIDFromCtx(ctx context.Context) slog.Attr {
	span := trace.SpanFromContext(ctx)
	traceId := ""

	if span != nil && span.SpanContext().HasTraceID() {
		traceId = span.SpanContext().TraceID().String()
	} else {
		traceId = ulid.Make().String()
	}

	return slog.Any(constant.LogFieldTraceId, traceId)
}

func UtilSpanError(span trace.Span, err error) {
	if err == nil {
		return
	}

	span.SetStatus(codes.Error, err.Error())
	span.RecordError(err)
}

func PublishMessage(ctx context.Context, publisher jetstream.Publisher, subject string, body any) error {
	ctx, span := otel.Tracer.Start(ctx, "publishMessage")
	defer span.End()

	traceIdAttr := ExtractTraceIDFromCtx(ctx)

	data, err := json.Marshal(body)
	if err != nil {
		slog.ErrorContext(ctx, "failed to marshal message", traceIdAttr, slog.Any(constant.LogFieldErr, err))
		UtilSpanError(span, err)
		return err
	}

	_, err = publisher.Publish(ctx, subject, data)
	if err != nil {
		slog.ErrorContext(ctx, "failed to publish message", traceIdAttr, slog.Any(constant.LogFieldErr, err))
		UtilSpanError(span, err)
		return err
	}

	return nil
}
