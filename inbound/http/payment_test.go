package http

import (
	"concert-ticket/common/constant"
	jetsteamMock "concert-ticket/common/jetstream/mocks"
	"fmt"
	"github.com/go-playground/validator/v10"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type PaymentHttpTestSuite struct {
	suite.Suite

	Validate  *validator.Validate
	Publisher *jetsteamMock.MockPublisher
}

func (s *PaymentHttpTestSuite) SetupTest() {
	ctrl := gomock.NewController(s.T())

	s.Validate = validator.New()
	s.Publisher = jetsteamMock.NewMockPublisher(ctrl)

	slog.SetLogLoggerLevel(slog.LevelDebug)
}

func TestPaymentHttpTestSuite(t *testing.T) {
	suite.Run(t, new(PaymentHttpTestSuite))
}

func (s *PaymentHttpTestSuite) TestCallback() {
	tests := []struct {
		name           string
		reqBody        string
		setupMock      func()
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "invalid json",
			reqBody:        `{invalid json`,
			setupMock:      func() {},
			expectedStatus: http.StatusBadRequest,
			expectedBody:   `{"error":"Invalid request"}`,
		},
		{
			name:           "validation error - missing external_id",
			reqBody:        `{}`,
			setupMock:      func() {},
			expectedStatus: http.StatusBadRequest,
			expectedBody:   `{"error":"Validation failed","data":{"ExternalId":"required"}}`,
		},
		{
			name:    "publish message error",
			reqBody: `{"external_id": "test-id-123"}`,
			setupMock: func() {
				s.Publisher.EXPECT().Publish(
					gomock.Any(),
					constant.SubjectCallbackPayment,
					gomock.Any(),
				).Return(nil, fmt.Errorf("publish error"))
			},
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   `{"error":"Internal Server Error"}`,
		},
		{
			name:    "success",
			reqBody: `{"external_id": "test-id-123"}`,
			setupMock: func() {
				s.Publisher.EXPECT().Publish(
					gomock.Any(),
					constant.SubjectCallbackPayment,
					gomock.Any(),
				).Return(nil, nil)
			},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tc := range tests {
		s.Run(tc.name, func() {
			paymentHttp := RegisterPaymentHttp(
				http.NewServeMux(),
				s.Publisher,
				s.Validate,
			)

			tc.setupMock()

			req := httptest.NewRequest(http.MethodPost, "/api/payments/callback", strings.NewReader(tc.reqBody))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			paymentHttp.callback(w, req)

			s.Equal(tc.expectedStatus, w.Code)

			if tc.expectedBody != "" {
				actual := strings.TrimSpace(w.Body.String())
				s.Equal(tc.expectedBody, actual)
			} else {
				s.Empty(w.Body.String())
			}
		})
	}
}
