package event

import (
	"concert-ticket/common"
	"concert-ticket/common/constant"
	"concert-ticket/common/contract"
	"concert-ticket/common/otel"
	"concert-ticket/model"
	"concert-ticket/outbound/sqlgen"
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/oklog/ulid/v2"
	"golang.org/x/text/message"
	"log/slog"
	"time"
)

type OrderEvent struct {
	Db                   contract.DbConn
	Querier              *sqlgen.Queries
	Publisher            jetstream.Publisher
	IdrCurrencyFormatter *message.Printer

	Timeout time.Duration
}

func (in OrderEvent) CreateHandler(ctx context.Context, msg []byte) error {
	ctx, cancel := context.WithTimeout(ctx, in.Timeout)
	defer cancel()

	var req model.CreateOrderEventMessage
	err := json.Unmarshal(msg, &req)
	if err != nil {
		slog.WarnContext(ctx, "create order event unmarshal error", slog.Any(constant.LogFieldErr, err))
		return nil
	}

	traceIdAttr := slog.String(constant.LogFieldTraceId, ulid.Make().String())
	reqAttr := slog.Any(constant.LogFieldPayload, string(msg))

	sendEmailReq := model.SendEmailEventMessage{
		To:      req.Email,
		Subject: "Order Confirmation",
		Body:    in.buildOrderConfirmationEmailBody(req),
	}

	err = common.PublishMessage(ctx, in.Publisher, constant.SubjectSendEmail, sendEmailReq)
	if err != nil {
		slog.ErrorContext(ctx, "create order event publish error", slog.Any(constant.LogFieldErr, err), reqAttr, traceIdAttr)
		return err
	}

	slog.DebugContext(ctx, "create order event publish success", reqAttr, traceIdAttr)

	return nil
}

func (in OrderEvent) buildOrderConfirmationEmailBody(req model.CreateOrderEventMessage) string {
	priceFormattedIdr := in.IdrCurrencyFormatter.Sprintf("Rp%d", constant.CategoryPriceById[req.CategoryID])
	categoryName := constant.CategoryNameById[req.CategoryID]

	return fmt.Sprintf(constant.EmailOrderConfirmationTemplate,
		req.Name,
		fmt.Sprintf("CLDPLY-%d", req.ID),
		categoryName,
		priceFormattedIdr,
		req.PaymentCode,
		req.ExpiredAt,
	)
}

func (in OrderEvent) CompleteHandler(ctx context.Context, msg []byte) error {
	ctx, cancel := context.WithTimeout(ctx, in.Timeout)
	defer cancel()

	var req model.PaymentCallbackRequest
	err := json.Unmarshal(msg, &req)
	if err != nil {
		slog.WarnContext(ctx, "complete order event unmarshal error", slog.Any(constant.LogFieldErr, err))
		return nil
	}

	ctx, span := otel.Tracer.Start(ctx, "OrderEvent.complete")
	defer span.End()

	traceIdAttr := common.ExtractTraceIDFromCtx(ctx)

	slog.InfoContext(ctx, "complete order event receive request", slog.Any(constant.LogFieldPayload, req), traceIdAttr)

	order, err := in.Querier.FindOrderByExternalIdAndStatusPending(ctx, req.ExternalId)
	if err != nil && err != pgx.ErrNoRows {
		slog.ErrorContext(ctx, "failed to get order", traceIdAttr, slog.Any(constant.LogFieldErr, err))
		return err
	}

	if err == pgx.ErrNoRows {
		slog.WarnContext(ctx, "order not found", traceIdAttr)
		return nil
	}

	cmd, err := in.Querier.UpdateOrderStatusToCompleted(ctx, order.ID)
	if err != nil {
		slog.ErrorContext(ctx, "failed to update order status", traceIdAttr, slog.Any(constant.LogFieldErr, err))
		return err
	}

	if cmd.RowsAffected() == 0 {
		slog.WarnContext(ctx, "order status is not pending", traceIdAttr)
		return nil
	}

	assignOrderTicketRowCol := model.AssignOrderTicketRowCol{
		ID:         order.ID,
		CategoryId: order.CategoryID,
		Email:      order.Email,
		Name:       order.Name,
	}

	err = common.PublishMessage(ctx, in.Publisher, constant.SubjectAssignOrderTicketRowCol, assignOrderTicketRowCol)
	if err != nil {
		slog.ErrorContext(ctx, "failed to publish assign order ticket row col message", traceIdAttr, slog.Any(constant.LogFieldErr, err))
		return err
	}

	slog.InfoContext(ctx, "order status updated to completed", traceIdAttr)

	return nil
}

func (in OrderEvent) AssignTicketColHandler(ctx context.Context, msg []byte) error {
	ctx, cancel := context.WithTimeout(ctx, in.Timeout)
	defer cancel()

	var req model.AssignOrderTicketRowCol
	err := json.Unmarshal(msg, &req)
	if err != nil {
		slog.WarnContext(ctx, "assign ticket col event unmarshal error", slog.Any(constant.LogFieldErr, err))
		return nil
	}

	ctx, span := otel.Tracer.Start(ctx, "OrderEvent.AssignTicketColHandler")
	defer span.End()

	traceIdAttr := common.ExtractTraceIDFromCtx(ctx)

	slog.InfoContext(ctx, "assign ticket col event receive request", slog.Any(constant.LogFieldPayload, req), traceIdAttr)

	tx, err := in.Db.Begin(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to begin transaction", traceIdAttr, slog.Any(constant.LogFieldErr, err))
		return err
	}

	defer func() {
		if err := tx.Rollback(ctx); err != nil && err != pgx.ErrTxClosed {
			slog.ErrorContext(ctx, "failed to rollback transaction", traceIdAttr, slog.Any(constant.LogFieldErr, err))
		}
	}()

	withTx := in.Querier.WithTx(tx)

	decrementedTicket, err := withTx.DecrementCategoryQuantityCol(ctx, req.CategoryId)
	if err != nil {
		slog.ErrorContext(ctx, "failed to decrement category quantity col", traceIdAttr, slog.Any(constant.LogFieldErr, err))
		return err
	}

	if decrementedTicket.Row == 0 {
		slog.ErrorContext(ctx, "category quantity row is 0", traceIdAttr)
		return fmt.Errorf("category quantity row is 0")
	}

	if decrementedTicket.Col < 1 {
		slog.ErrorContext(ctx, "category quantity col is 0", traceIdAttr)
		return fmt.Errorf("category quantity col is 0")
	}

	cmd, err := withTx.UpdateOrderTicketRowCol(ctx, sqlgen.UpdateOrderTicketRowColParams{
		ID:        req.ID,
		TicketRow: pgtype.Int4{Int32: decrementedTicket.Row, Valid: true},
		TicketCol: pgtype.Int4{Int32: decrementedTicket.Col, Valid: true},
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to update order ticket row col", traceIdAttr, slog.Any(constant.LogFieldErr, err))
		return err
	}

	if cmd.RowsAffected() == 0 {
		slog.ErrorContext(ctx, "order ticket row col is not updated", traceIdAttr)
		return fmt.Errorf("order ticket row col is not updated")
	}

	err = tx.Commit(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to commit transaction", traceIdAttr, slog.Any(constant.LogFieldErr, err))
		return err
	}

	emailPayload := model.SendEmailEventMessage{
		To:      req.Email,
		Subject: "Order Confirmation",
		Body:    in.buildOrderCompletionEmailBody(req, decrementedTicket.Row, decrementedTicket.Col),
	}

	err = common.PublishMessage(ctx, in.Publisher, constant.SubjectSendEmail, emailPayload)
	if err != nil {
		slog.ErrorContext(ctx, "failed to publish email payload", traceIdAttr, slog.Any(constant.LogFieldErr, err))
		return err
	}

	slog.InfoContext(ctx, "assign ticket col event success", traceIdAttr)

	return nil
}

func (in OrderEvent) buildOrderCompletionEmailBody(req model.AssignOrderTicketRowCol, row, col int32) string {
	priceFormattedIdr := in.IdrCurrencyFormatter.Sprintf("Rp%d", constant.CategoryPriceById[req.CategoryId])
	categoryName := constant.CategoryNameById[req.CategoryId]
	orderID := fmt.Sprintf("CLDPLY-%d", req.ID)

	return fmt.Sprintf(constant.EmailOrderCompletionTemplate, req.Name, orderID, categoryName, priceFormattedIdr, row, col)
}
