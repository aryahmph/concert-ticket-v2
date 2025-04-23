package http

import (
	"concert-ticket/common/errs"
	"concert-ticket/model"
	"encoding/json"
	"github.com/go-playground/validator/v10"
	"github.com/oklog/ulid/v2"
	"net/http"
	"time"
)

func writeJSONResponse(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if data == nil {
		return
	}

	if err := json.NewEncoder(w).Encode(data); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func writeErrorResponse(w http.ResponseWriter, err error) {
	if err == nil {
		return
	}

	w.Header().Set("Content-Type", "application/json")

	var message string
	var data any
	if httpErr, ok := err.(*errs.HttpError); ok {
		message = httpErr.Message
		data = httpErr.Data
		w.WriteHeader(httpErr.Code)
	} else if validationErr, ok := err.(validator.ValidationErrors); ok {
		message = "Validation failed"
		w.WriteHeader(http.StatusBadRequest)

		validationErrors := make(map[string]string)
		for _, fieldErr := range validationErr {
			fieldName := fieldErr.Field()
			validationErrors[fieldName] = fieldErr.Tag()
		}

		data = validationErrors
	} else {
		message = "Internal Server Error"
		w.WriteHeader(500)
	}

	errorResponse := model.ErrorResponse{Error: message, Data: data}
	if err := json.NewEncoder(w).Encode(errorResponse); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func generateDummyPaymentCode(externalId string, price int64) string {
	time.Sleep(10 * time.Millisecond)
	return ulid.Make().String()
}
