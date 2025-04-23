package http

import (
	"concert-ticket/common"
	"concert-ticket/common/constant"
	"concert-ticket/common/errs"
	"concert-ticket/model"
	"encoding/json"
	"github.com/go-playground/validator/v10"
	"github.com/nats-io/nats.go/jetstream"
	"log/slog"
	"net/http"
)

type PaymentHttp struct {
	Publisher jetstream.Publisher
	Validate  *validator.Validate
}

func RegisterPaymentHttp(
	mux *http.ServeMux,
	publisher jetstream.Publisher,
	validate *validator.Validate,
) *PaymentHttp {
	in := &PaymentHttp{
		Publisher: publisher,
		Validate:  validate,
	}

	mux.HandleFunc("POST /api/payments/callback", in.callback)

	return in
}

func (in PaymentHttp) callback(w http.ResponseWriter, r *http.Request) {
	var req model.PaymentCallbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorResponse(w, &errs.HttpError{Code: http.StatusBadRequest, Message: "Invalid request"})
		return
	}

	if err := in.Validate.Struct(req); err != nil {
		writeErrorResponse(w, err)
		return
	}

	ctx := r.Context()
	err := common.PublishMessage(ctx, in.Publisher, constant.SubjectCallbackPayment, model.PaymentCallbackRequest{ExternalId: req.ExternalId})
	if err != nil {
		slog.ErrorContext(ctx, "error publish message when callback payment", slog.Any(constant.LogFieldErr, err))
		writeErrorResponse(w, err)
		return
	}

	w.WriteHeader(http.StatusOK)
}
