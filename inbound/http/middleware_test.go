package http

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type MiddlewareTestSuite struct {
	suite.Suite
}

func TestMiddlewareTestSuite(t *testing.T) {
	suite.Run(t, new(MiddlewareTestSuite))
}

func (s *MiddlewareTestSuite) TestCorsMiddleware() {
	tests := []struct {
		name            string
		method          string
		expectedStatus  int
		expectedHeaders map[string]string
		handlerCalled   bool
	}{
		{
			name:           "OPTIONS request",
			method:         http.MethodOptions,
			expectedStatus: http.StatusOK,
			expectedHeaders: map[string]string{
				"Access-Control-Allow-Origin":  "*",
				"Access-Control-Allow-Methods": "GET, POST, PUT, DELETE, OPTIONS",
				"Access-Control-Allow-Headers": "Content-Type, Authorization",
			},
			handlerCalled: false,
		},
		{
			name:           "GET request",
			method:         http.MethodGet,
			expectedStatus: http.StatusOK,
			expectedHeaders: map[string]string{
				"Access-Control-Allow-Origin":  "*",
				"Access-Control-Allow-Methods": "GET, POST, PUT, DELETE, OPTIONS",
				"Access-Control-Allow-Headers": "Content-Type, Authorization",
			},
			handlerCalled: true,
		},
	}

	for _, tc := range tests {
		s.Run(tc.name, func() {
			handlerCalled := false
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				handlerCalled = true
				w.WriteHeader(http.StatusOK)
			})

			middleware := CorsMiddleware(handler)

			req := httptest.NewRequest(tc.method, "/test", nil)
			w := httptest.NewRecorder()

			middleware.ServeHTTP(w, req)

			s.Equal(tc.expectedStatus, w.Code)
			for key, value := range tc.expectedHeaders {
				s.Equal(value, w.Header().Get(key))
			}
			s.Equal(tc.handlerCalled, handlerCalled)
		})
	}
}

func (s *MiddlewareTestSuite) TestTimeoutMiddleware() {
	tests := []struct {
		name           string
		handlerDelay   time.Duration
		timeout        time.Duration
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "request completes in time",
			handlerDelay:   1 * time.Millisecond,
			timeout:        100 * time.Millisecond,
			expectedStatus: http.StatusOK,
			expectedBody:   "success",
		},
		{
			name:           "request times out",
			handlerDelay:   200 * time.Millisecond,
			timeout:        50 * time.Millisecond,
			expectedStatus: http.StatusServiceUnavailable,
			expectedBody:   "request timeout",
		},
	}

	for _, tc := range tests {
		s.Run(tc.name, func() {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				time.Sleep(tc.handlerDelay)
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("success"))
			})

			middleware := TimeoutMiddleware(tc.timeout)(handler)

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			w := httptest.NewRecorder()

			middleware.ServeHTTP(w, req)

			s.Equal(tc.expectedStatus, w.Code, "Expected status code %d but got %d", tc.expectedStatus, w.Code)
			s.Contains(w.Body.String(), tc.expectedBody)
		})
	}
}
