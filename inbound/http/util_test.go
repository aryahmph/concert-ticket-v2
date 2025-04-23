package http

import (
	"concert-ticket/common/errs"
	"encoding/json"
	"errors"
	"github.com/go-playground/validator/v10"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteJSONResponse(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		data           interface{}
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "success with data",
			statusCode:     http.StatusOK,
			data:           map[string]interface{}{"key": "value"},
			expectedStatus: http.StatusOK,
			expectedBody:   `{"key":"value"}`,
		},
		{
			name:           "success with nil data",
			statusCode:     http.StatusCreated,
			data:           nil,
			expectedStatus: http.StatusCreated,
			expectedBody:   "",
		},
		{
			name:           "success with empty struct",
			statusCode:     http.StatusAccepted,
			data:           struct{}{},
			expectedStatus: http.StatusAccepted,
			expectedBody:   `{}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()

			writeJSONResponse(w, tc.statusCode, tc.data)

			assert.Equal(t, tc.expectedStatus, w.Code)
			assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

			body := strings.TrimSpace(w.Body.String())
			assert.Equal(t, tc.expectedBody, body)
		})
	}
}

func TestWriteErrorResponse(t *testing.T) {
	validate := validator.New()

	type testStruct struct {
		Name  string `validate:"required"`
		Email string `validate:"email"`
	}

	invalidStruct := testStruct{Name: "", Email: "invalid"}
	validationErr := validate.Struct(invalidStruct)

	tests := []struct {
		name           string
		err            error
		expectedStatus int
		expectedBody   string
		checkFields    func(t *testing.T, body map[string]interface{})
	}{
		{
			name:           "nil error",
			err:            nil,
			expectedStatus: http.StatusOK,
			expectedBody:   "",
		},
		{
			name:           "http error",
			err:            &errs.HttpError{Code: http.StatusNotFound, Message: "Not Found"},
			expectedStatus: http.StatusNotFound,
			expectedBody:   `{"error":"Not Found"}`,
		},
		{
			name:           "validation error",
			err:            validationErr,
			expectedStatus: http.StatusBadRequest,
			checkFields: func(t *testing.T, body map[string]interface{}) {
				assert.Equal(t, "Validation failed", body["error"])
				data, ok := body["data"].(map[string]interface{})
				require.True(t, ok)
				assert.Contains(t, data, "Name")
				assert.Contains(t, data, "Email")
			},
		},
		{
			name:           "generic error",
			err:            errors.New("something went wrong"),
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   `{"error":"Internal Server Error"}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()

			writeErrorResponse(w, tc.err)

			if tc.err == nil {
				assert.Empty(t, w.Body.String())
				return
			}

			assert.Equal(t, tc.expectedStatus, w.Code)
			assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

			if tc.expectedBody != "" {
				body := strings.TrimSpace(w.Body.String())
				assert.Equal(t, tc.expectedBody, body)
			}

			if tc.checkFields != nil {
				var responseBody map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &responseBody)
				require.NoError(t, err)
				tc.checkFields(t, responseBody)
			}
		})
	}
}
