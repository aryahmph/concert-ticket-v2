package http

import (
	"concert-ticket/common"
	"concert-ticket/common/constant"
	"concert-ticket/common/errs"
	"concert-ticket/common/otel"
	"concert-ticket/model"
	"concert-ticket/outbound/sqlgen"
	"encoding/json"
	"fmt"
	"github.com/go-playground/validator/v10"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/oklog/ulid/v2"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"
	"golang.org/x/text/message"
	"log/slog"
	"net/http"
	"time"
)

type OrderHttp struct {
	Querier              *sqlgen.Queries
	Cache                *redis.Client
	Publisher            jetstream.Publisher
	Validate             *validator.Validate
	IdrCurrencyFormatter *message.Printer

	TimeNow func() time.Time

	sizeBulkCancel int32
	expiredAfter   time.Duration
}

func RegisterOrderHttp(
	mux *http.ServeMux,
	cfg *viper.Viper,
	querier *sqlgen.Queries,
	cache *redis.Client,
	publisher jetstream.Publisher,
	validate *validator.Validate,
	idrCurrencyFormatter *message.Printer,
) *OrderHttp {
	in := &OrderHttp{
		Querier:              querier,
		Cache:                cache,
		Publisher:            publisher,
		Validate:             validate,
		IdrCurrencyFormatter: idrCurrencyFormatter,
		TimeNow:              time.Now,

		sizeBulkCancel: cfg.GetInt32("order.bulk_cancel_size"),
		expiredAfter:   cfg.GetDuration("order.expired_after"),
	}

	mux.HandleFunc("POST /api/orders", in.create)
	mux.HandleFunc("POST /api/orders/cancel", in.cancel)

	return in
}

func (in OrderHttp) create(w http.ResponseWriter, r *http.Request) {
	var req model.CreateOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorResponse(w, &errs.HttpError{Code: http.StatusBadRequest, Message: "Invalid request"})
		return
	}

	if err := in.validateCreateOrderRequest(req); err != nil {
		writeErrorResponse(w, err)
		return
	}

	ctx, span := otel.Tracer.Start(r.Context(), "OrderHttp.create")
	defer span.End()

	traceIdAttr := common.ExtractTraceIDFromCtx(ctx)
	slog.InfoContext(ctx, "create order receive request", slog.Any(constant.LogFieldPayload, req), traceIdAttr)

	emailLock, err := in.Cache.SetNX(ctx, fmt.Sprintf(constant.OrderEmailLock, req.Email), true, constant.OrderEmailLockDefaultTTL).Result()
	if err != nil {
		slog.ErrorContext(ctx, "failed to set email lock", traceIdAttr, slog.Any(constant.LogFieldErr, err))
		writeErrorResponse(w, err)
		return
	}

	if !emailLock {
		slog.DebugContext(ctx, "email already ordered", traceIdAttr)
		writeErrorResponse(w, &errs.HttpError{Code: http.StatusConflict, Message: "Email already ordered"})
		return
	}

	emailExist, err := in.Querier.FindOrderByEmailAndStatusPending(ctx, req.Email)
	if err != nil {
		slog.ErrorContext(ctx, "failed to find order by email", traceIdAttr, slog.Any(constant.LogFieldErr, err))
		writeErrorResponse(w, err)
		return
	}

	if emailExist {
		slog.DebugContext(ctx, "email already ordered", traceIdAttr)
		writeErrorResponse(w, &errs.HttpError{Code: http.StatusConflict, Message: "Email already ordered"})
		return
	}

	atomicVal, err := in.Cache.Decr(ctx, fmt.Sprintf(constant.EachCategoryQuantityKey, req.CategoryId)).Result()
	if err != nil {
		slog.ErrorContext(ctx, "failed to decrement category quantity", traceIdAttr, slog.Any(constant.LogFieldErr, err))
		writeErrorResponse(w, err)
		return
	}

	if atomicVal < 0 {
		slog.DebugContext(ctx, "category sold out", traceIdAttr)

		redisErr := in.Cache.Incr(ctx, fmt.Sprintf(constant.EachCategoryQuantityKey, req.CategoryId)).Err()
		if redisErr != nil {
			slog.ErrorContext(ctx, "failed to increment category quantity", traceIdAttr, slog.Any(constant.LogFieldErr, redisErr))
		}

		writeErrorResponse(w, &errs.HttpError{Code: http.StatusConflict, Message: "Category sold out"})
		return
	}

	err = common.PublishMessage(ctx, in.Publisher, constant.SubjectIncrementCategoryQuantity, model.IncrementCategoryQuantityEventMessage{
		ID:       req.CategoryId,
		Quantity: -1,
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to publish increment category quantity message", traceIdAttr, slog.Any(constant.LogFieldErr, err))
		writeErrorResponse(w, err)
		return
	}

	defer func() {
		if err != nil {
			err2 := common.PublishMessage(ctx, in.Publisher, constant.SubjectIncrementCategoryQuantity, model.IncrementCategoryQuantityEventMessage{
				ID:       req.CategoryId,
				Quantity: 1,
			})
			if err2 != nil {
				slog.ErrorContext(ctx, "failed to publish increment category quantity message", traceIdAttr, slog.Any(constant.LogFieldErr, err2))
			}
		}
	}()

	externalId := ulid.Make().String()
	price, _ := constant.CategoryPriceById[req.CategoryId]
	vaCode := generateDummyPaymentCode(externalId, price)

	expiredAt := in.TimeNow().Add(in.expiredAfter)
	returnId, err := in.Querier.InsertOrder(ctx, sqlgen.InsertOrderParams{
		CategoryID:  req.CategoryId,
		ExternalID:  externalId,
		Name:        req.Name,
		Email:       req.Email,
		PaymentCode: vaCode,
		ExpiredAt:   pgtype.Timestamp{Time: expiredAt, Valid: true},
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to insert order", traceIdAttr, slog.Any(constant.LogFieldErr, err))
		writeErrorResponse(w, err)
		return
	}

	err = common.PublishMessage(ctx, in.Publisher, constant.SubjectCreateOrder, model.CreateOrderEventMessage{
		ID:          returnId,
		CategoryID:  req.CategoryId,
		ExternalID:  externalId,
		Name:        req.Name,
		Email:       req.Email,
		PaymentCode: vaCode,
		ExpiredAt:   expiredAt.Format(time.RFC3339),
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to publish create order message", traceIdAttr, slog.Any(constant.LogFieldErr, err))
		writeErrorResponse(w, err)
		return
	}

	slog.InfoContext(ctx, "insert order success", traceIdAttr, slog.Any(constant.LogFieldResponse, returnId))

	writeJSONResponse(w, http.StatusOK, model.CreateOrderResponse{
		Id:          returnId,
		ExternalId:  externalId,
		PaymentCode: vaCode,
	})
}

func (in OrderHttp) cancel(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer.Start(r.Context(), "OrderHttp.cancel")
	defer span.End()

	traceIdAttr := common.ExtractTraceIDFromCtx(ctx)
	slog.InfoContext(ctx, "cancel order receive request", traceIdAttr)

	cancelableOrders, err := in.Querier.BulkCancelOrders(ctx, sqlgen.BulkCancelOrdersParams{
		Limit:     in.sizeBulkCancel,
		UpdatedAt: pgtype.Timestamp{Time: in.TimeNow(), Valid: true},
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to find cancelable orders", traceIdAttr, slog.Any(constant.LogFieldErr, err))
		writeErrorResponse(w, err)
		return
	}

	if len(cancelableOrders) == 0 {
		slog.DebugContext(ctx, "no cancelable orders", traceIdAttr)
		writeJSONResponse(w, http.StatusOK, nil)
		return
	}

	categoryIdValMap := make(map[int16]int32)
	for _, order := range cancelableOrders {
		categoryIdValMap[order.CategoryID]++
	}

	pipeline := in.Cache.Pipeline()
	for categoryId, val := range categoryIdValMap {
		pipeline.IncrBy(ctx, fmt.Sprintf(constant.EachCategoryQuantityKey, categoryId), int64(val))
	}

	_, err = pipeline.Exec(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to increment category quantity", traceIdAttr, slog.Any(constant.LogFieldErr, err))
		writeErrorResponse(w, err)
		return
	}

	incrementPayload := make([]model.IncrementCategoryQuantityEventMessage, 0, len(categoryIdValMap))
	for categoryId, val := range categoryIdValMap {
		incrementPayload = append(incrementPayload, model.IncrementCategoryQuantityEventMessage{
			ID:       categoryId,
			Quantity: val,
		})
	}

	err = common.PublishMessage(ctx, in.Publisher, constant.SubjectBulkIncrementCategoryQuantity, incrementPayload)
	if err != nil {
		slog.ErrorContext(ctx, "failed to publish bulk increment category quantity message", traceIdAttr, slog.Any(constant.LogFieldErr, err))
		writeErrorResponse(w, err)
		return
	}

	for _, order := range cancelableOrders {
		err = common.PublishMessage(ctx, in.Publisher, constant.SubjectSendEmail, model.SendEmailEventMessage{
			To:      order.Email,
			Subject: "Order Cancellation",
			Body:    in.buildOrderCancellationEmailBody(order),
		})
		if err != nil {
			slog.ErrorContext(ctx, "failed to publish cancel order message", traceIdAttr, slog.Any(constant.LogFieldErr, err))
			writeErrorResponse(w, err)
			return
		}
	}

	slog.InfoContext(ctx, "cancel order success", slog.Any(constant.LogFieldResponse, len(cancelableOrders)), traceIdAttr)

	writeJSONResponse(w, http.StatusOK, nil)
}

func (in OrderHttp) validateCreateOrderRequest(req model.CreateOrderRequest) error {
	if err := in.Validate.Struct(req); err != nil {
		return err
	}

	_, ok := constant.CategoryPriceById[req.CategoryId]
	if !ok {
		return &errs.HttpError{
			Code:    http.StatusBadRequest,
			Message: "Validation failed",
			Data: map[string]any{
				"CategoryId": "not found",
			},
		}
	}

	return nil
}

func (in OrderHttp) buildOrderCancellationEmailBody(row sqlgen.BulkCancelOrdersRow) string {
	priceFormattedIdr := in.IdrCurrencyFormatter.Sprintf("Rp%d", constant.CategoryPriceById[row.CategoryID])
	categoryName := constant.CategoryNameById[row.CategoryID]
	orderID := fmt.Sprintf("CLDPLY-%d", row.ID)

	return fmt.Sprintf(constant.EmailOrderCancellationTemplate, row.Name, orderID, categoryName, priceFormattedIdr)
}
