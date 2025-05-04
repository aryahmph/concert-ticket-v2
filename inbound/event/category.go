package event

import (
	"concert-ticket/common"
	"concert-ticket/common/constant"
	"concert-ticket/common/otel"
	"concert-ticket/model"
	"concert-ticket/outbound/sqlgen"
	"context"
	"encoding/json"
	"log/slog"
	"time"
)

type CategoryEvent struct {
	Querier *sqlgen.Queries
	Timeout time.Duration
}

func (in CategoryEvent) BulkIncrementCategoryQuantityHandler(ctx context.Context, msg []byte) error {
	ctx, cancel := context.WithTimeout(ctx, in.Timeout)
	defer cancel()

	var req []model.IncrementCategoryQuantityEventMessage
	err := json.Unmarshal(msg, &req)
	if err != nil {
		slog.WarnContext(ctx, "bulk increment category quantity event unmarshal error", slog.Any(constant.LogFieldErr, err))
		return nil
	}

	ctx, span := otel.Tracer.Start(ctx, "CategoryEvent.BulkIncrementCategoryQuantityHandler")
	defer span.End()

	traceIdAttr := common.ExtractTraceIDFromCtx(ctx)

	slog.InfoContext(ctx, "bulk increment category quantity event receive request", slog.Any(constant.LogFieldPayload, req), traceIdAttr)

	categoryIdValueMap := make(map[int16]int32)
	for _, category := range req {
		categoryIdValueMap[category.ID] += category.Quantity
	}

	err = in.Querier.BulkIncrementCategoryQuantity(ctx, sqlgen.BulkIncrementCategoryQuantityParams{
		Column1: categoryIdValueMap[1],
		Column2: categoryIdValueMap[2],
		Column3: categoryIdValueMap[3],
		Column4: categoryIdValueMap[4],
		Column5: categoryIdValueMap[5],
		Column6: categoryIdValueMap[6],
		Column7: categoryIdValueMap[7],
		Column8: categoryIdValueMap[8],
		Column9: categoryIdValueMap[9],
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to bulk increment category quantity", traceIdAttr, slog.Any(constant.LogFieldErr, err))
		return err
	}

	slog.InfoContext(ctx, "bulk increment category quantity event success", traceIdAttr)
	return nil
}

func (in CategoryEvent) IncrementCategoryQuantityHandler(ctx context.Context, msg []byte) model.IncrementCategoryQuantityEventMessage {
	var req model.IncrementCategoryQuantityEventMessage
	err := json.Unmarshal(msg, &req)
	if err != nil {
		slog.WarnContext(ctx, "increment category quantity event unmarshal error", slog.Any(constant.LogFieldErr, err))
		return req
	}

	return req
}
