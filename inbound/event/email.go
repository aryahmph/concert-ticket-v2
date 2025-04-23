package event

import (
	"concert-ticket/common/constant"
	"concert-ticket/model"
	emailOutbound "concert-ticket/outbound/email"
	"context"
	"encoding/json"
	"github.com/oklog/ulid/v2"
	"log/slog"
	"time"
)

type EmailEvent struct {
	EmailOutbound emailOutbound.EmailOutbound
	Timeout       time.Duration
}

func (in EmailEvent) SendEmailHandler(ctx context.Context, msg []byte) error {
	ctx, cancel := context.WithTimeout(ctx, in.Timeout)
	defer cancel()

	var req model.SendEmailEventMessage
	err := json.Unmarshal(msg, &req)
	if err != nil {
		slog.WarnContext(ctx, "create order event unmarshal error", slog.Any(constant.LogFieldErr, err))
		return nil
	}

	traceIdAttr := slog.String(constant.LogFieldTraceId, ulid.Make().String())
	reqAttr := slog.Any(constant.LogFieldPayload, string(msg))

	err = in.EmailOutbound.Send([]string{req.To}, req.Subject, req.Body)
	if err != nil {
		slog.ErrorContext(ctx, "send email event publish error", slog.Any(constant.LogFieldErr, err), reqAttr, traceIdAttr)
		return err
	}

	return nil
}
