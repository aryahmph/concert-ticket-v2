package http

import (
	"concert-ticket/common/constant"
	jetsteamMock "concert-ticket/common/jetstream/mocks"
	"concert-ticket/outbound/sqlgen"
	"fmt"
	"github.com/go-playground/validator/v10"
	"github.com/go-redis/redismock/v9"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type OrderHttpTestSuite struct {
	suite.Suite

	Cfg *viper.Viper

	Querier *sqlgen.Queries
	PgxMock pgxmock.PgxPoolIface

	Cache     *redis.Client
	CacheMock redismock.ClientMock

	Validate  *validator.Validate
	Publisher *jetsteamMock.MockPublisher
}

func (s *OrderHttpTestSuite) SetupTest() {
	ctrl := gomock.NewController(s.T())

	rdb, mock := redismock.NewClientMock()
	s.Cache = rdb
	s.CacheMock = mock

	pool, err := pgxmock.NewPool()
	if err != nil {
		s.T().Fatalf("failed to create pgxmock pool: %v", err)
	}

	s.PgxMock = pool
	s.Querier = sqlgen.New(pool)

	s.Validate = validator.New()
	s.Publisher = jetsteamMock.NewMockPublisher(ctrl)

	s.Cfg = viper.New()
	s.Cfg.Set("order.expired_after", "15m")
	s.Cfg.Set("order.bulk_cancel_size", 10)

	slog.SetLogLoggerLevel(slog.LevelDebug)
}

func (s *OrderHttpTestSuite) TearDownTest() {
	s.PgxMock.Close()

	if err := s.Cache.Close(); err != nil {
		s.T().Fatalf("failed to close redis mock: %v", err)
	}
}

func TestOrderHttpTestSuite(t *testing.T) {
	suite.Run(t, new(OrderHttpTestSuite))
}

func (s *OrderHttpTestSuite) TestCreate() {
	tests := []struct {
		name           string
		reqBody        string
		setupMock      func()
		expectedStatus int
		expectedBody   string
		isTestBody     bool
		timeNow        func() time.Time
	}{
		{
			name:           "invalid json",
			reqBody:        `{invalid json`,
			setupMock:      func() {},
			expectedStatus: http.StatusBadRequest,
			expectedBody:   `{"error":"Invalid request"}`,
		},
		{
			name:           "validation error - invalid category #1",
			reqBody:        `{"name": "John Doe", "email": "john@example.com", "phone": "+6281234567890"}`,
			setupMock:      func() {},
			expectedStatus: http.StatusBadRequest,
			expectedBody:   `{"error":"Validation failed","data":{"CategoryId":"required"}}`,
		},
		{
			name:           "validation error - invalid category #2",
			reqBody:        `{"category_id": 99, "name": "John Doe", "email": "john@example.com", "phone": "+6281234567890"}`,
			setupMock:      func() {},
			expectedStatus: http.StatusBadRequest,
			expectedBody:   `{"error":"Validation failed","data":{"CategoryId":"not found"}}`,
		},
		{
			name:           "validation error - invalid phone",
			reqBody:        `{"category_id": 1, "name": "John Doe", "email": "john@example.com", "phone": "invalid"}`,
			setupMock:      func() {},
			expectedStatus: http.StatusBadRequest,
			expectedBody:   `{"error":"Validation failed","data":{"Phone":"not valid"}}`,
		},
		{
			name:    "email lock error",
			reqBody: `{"category_id": 1, "name": "John Doe", "email": "john@example.com", "phone": "+6281234567890"}`,
			setupMock: func() {
				s.CacheMock.ExpectSetNX(fmt.Sprintf(constant.OrderEmailLock, "john@example.com"), true, constant.OrderEmailLockDefaultTTL).
					SetErr(redis.ErrClosed)
			},
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   `{"error":"Internal Server Error"}`,
		},
		{
			name:    "email already ordered - from cache",
			reqBody: `{"category_id": 1, "name": "John Doe", "email": "john@example.com", "phone": "+6281234567890"}`,
			setupMock: func() {
				s.CacheMock.ExpectSetNX(fmt.Sprintf(constant.OrderEmailLock, "john@example.com"), true, constant.OrderEmailLockDefaultTTL).
					SetVal(false)
			},
			expectedStatus: http.StatusConflict,
			expectedBody:   `{"error":"Email already ordered"}`,
		},
		{
			name:    "check email from db error",
			reqBody: `{"category_id": 1, "name": "John Doe", "email": "john@example.com", "phone": "+6281234567890"}`,
			setupMock: func() {
				s.CacheMock.ExpectSetNX(fmt.Sprintf(constant.OrderEmailLock, "john@example.com"), true, constant.OrderEmailLockDefaultTTL).
					SetVal(true)
				s.PgxMock.ExpectQuery(`SELECT EXISTS \(SELECT 1 FROM orders WHERE email = \$1 AND status = 'pending'\) AS "exists"`).
					WithArgs("john@example.com").
					WillReturnError(fmt.Errorf("database error"))
			},
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   `{"error":"Internal Server Error"}`,
		},
		{
			name:    "check email already ordered",
			reqBody: `{"category_id": 1, "name": "John Doe", "email": "john@example.com", "phone": "+6281234567890"}`,
			setupMock: func() {
				s.CacheMock.ExpectSetNX(fmt.Sprintf(constant.OrderEmailLock, "john@example.com"), true, constant.OrderEmailLockDefaultTTL).
					SetVal(true)
				s.PgxMock.ExpectQuery(`SELECT EXISTS \(SELECT 1 FROM orders WHERE email = \$1 AND status = 'pending'\) AS "exists"`).
					WithArgs("john@example.com").
					WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
			},
			expectedStatus: http.StatusConflict,
			expectedBody:   `{"error":"Email already ordered"}`,
		},
		{
			name:    "decrement category error",
			reqBody: `{"category_id": 1, "name": "John Doe", "email": "john@example.com", "phone": "+6281234567890"}`,
			setupMock: func() {
				s.CacheMock.ExpectSetNX(fmt.Sprintf(constant.OrderEmailLock, "john@example.com"), true, constant.OrderEmailLockDefaultTTL).
					SetVal(true)
				s.PgxMock.ExpectQuery(`SELECT EXISTS \(SELECT 1 FROM orders WHERE email = \$1 AND status = 'pending'\) AS "exists"`).
					WithArgs("john@example.com").
					WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))
				s.CacheMock.ExpectDecr(fmt.Sprintf(constant.EachCategoryQuantityKey, int16(1))).
					SetErr(redis.ErrClosed)
			},
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   `{"error":"Internal Server Error"}`,
		},
		{
			name:    "increment category error",
			reqBody: `{"category_id": 1, "name": "John Doe", "email": "john@example.com", "phone": "+6281234567890"}`,
			setupMock: func() {
				s.CacheMock.ExpectSetNX(fmt.Sprintf(constant.OrderEmailLock, "john@example.com"), true, constant.OrderEmailLockDefaultTTL).
					SetVal(true)
				s.PgxMock.ExpectQuery(`SELECT EXISTS \(SELECT 1 FROM orders WHERE email = \$1 AND status = 'pending'\) AS "exists"`).
					WithArgs("john@example.com").
					WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))
				s.CacheMock.ExpectDecr(fmt.Sprintf(constant.EachCategoryQuantityKey, int16(1))).
					SetVal(-1)
				s.CacheMock.ExpectIncr(fmt.Sprintf(constant.EachCategoryQuantityKey, int16(1))).
					SetErr(redis.ErrClosed)
			},
			expectedStatus: http.StatusConflict,
			expectedBody:   `{"error":"Category sold out"}`,
		},
		{
			name:    "category sold out",
			reqBody: `{"category_id": 1, "name": "John Doe", "email": "john@example.com", "phone": "+6281234567890"}`,
			setupMock: func() {
				s.CacheMock.ExpectSetNX(fmt.Sprintf(constant.OrderEmailLock, "john@example.com"), true, constant.OrderEmailLockDefaultTTL).
					SetVal(true)
				s.PgxMock.ExpectQuery(`SELECT EXISTS \(SELECT 1 FROM orders WHERE email = \$1 AND status = 'pending'\) AS "exists"`).
					WithArgs("john@example.com").
					WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))
				s.CacheMock.ExpectDecr(fmt.Sprintf(constant.EachCategoryQuantityKey, int16(1))).
					SetVal(-1)
				s.CacheMock.ExpectIncr(fmt.Sprintf(constant.EachCategoryQuantityKey, int16(1))).
					SetVal(0)
			},
			expectedStatus: http.StatusConflict,
			expectedBody:   `{"error":"Category sold out"}`,
		},
		{
			name:    "publish message error - increment category",
			reqBody: `{"category_id": 1, "name": "John Doe", "email": "john@example.com", "phone": "+6281234567890"}`,
			setupMock: func() {
				s.CacheMock.ExpectSetNX(fmt.Sprintf(constant.OrderEmailLock, "john@example.com"), true, constant.OrderEmailLockDefaultTTL).
					SetVal(true)
				s.PgxMock.ExpectQuery(`SELECT EXISTS \(SELECT 1 FROM orders WHERE email = \$1 AND status = 'pending'\) AS "exists"`).
					WithArgs("john@example.com").
					WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))
				s.CacheMock.ExpectDecr(fmt.Sprintf(constant.EachCategoryQuantityKey, int16(1))).
					SetVal(0)

				s.Publisher.EXPECT().Publish(
					gomock.Any(),
					constant.SubjectIncrementCategoryQuantity,
					gomock.Any(),
				).Return(nil, fmt.Errorf("publish error"))
			},
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   `{"error":"Internal Server Error"}`,
		},
		{
			name:    "create order error",
			reqBody: `{"category_id": 1, "name": "John Doe", "email": "john@example.com", "phone": "+6281234567890"}`,
			setupMock: func() {
				s.CacheMock.ExpectSetNX(fmt.Sprintf(constant.OrderEmailLock, "john@example.com"), true, constant.OrderEmailLockDefaultTTL).
					SetVal(true)
				s.PgxMock.ExpectQuery(`SELECT EXISTS \(SELECT 1 FROM orders WHERE email = \$1 AND status = 'pending'\) AS "exists"`).
					WithArgs("john@example.com").
					WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))
				s.CacheMock.ExpectDecr(fmt.Sprintf(constant.EachCategoryQuantityKey, int16(1))).
					SetVal(0)

				fixedTime := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
				expiredAt := fixedTime.Add(15 * time.Minute)

				s.PgxMock.ExpectQuery("INSERT INTO orders").
					WithArgs(
						int16(1),           // category_id
						pgxmock.AnyArg(),   // external_id
						"John Doe",         // name
						"john@example.com", // email
						"+6281234567890",   // phone
						pgxmock.AnyArg(),   // payment_code
						pgtype.Timestamp{Time: expiredAt, Valid: true}, // expired_at with fixed time
					).
					WillReturnError(fmt.Errorf("database error"))

				s.Publisher.EXPECT().Publish(
					gomock.Any(),
					constant.SubjectIncrementCategoryQuantity,
					gomock.Any(),
				).Return(nil, nil).Times(2)
			},
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   `{"error":"Internal Server Error"}`,
			timeNow: func() time.Time {
				return time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
			},
		},
		{
			name:    "publish message error - create order",
			reqBody: `{"category_id": 1, "name": "John Doe", "email": "john@example.com", "phone": "+6281234567890"}`,
			setupMock: func() {
				s.CacheMock.ExpectSetNX(fmt.Sprintf(constant.OrderEmailLock, "john@example.com"), true, constant.OrderEmailLockDefaultTTL).
					SetVal(true)
				s.PgxMock.ExpectQuery(`SELECT EXISTS \(SELECT 1 FROM orders WHERE email = \$1 AND status = 'pending'\) AS "exists"`).
					WithArgs("john@example.com").
					WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))
				s.CacheMock.ExpectDecr(fmt.Sprintf(constant.EachCategoryQuantityKey, int16(1))).
					SetVal(0)

				fixedTime := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
				expiredAt := fixedTime.Add(15 * time.Minute)

				s.PgxMock.ExpectQuery("INSERT INTO orders").
					WithArgs(
						int16(1),           // category_id
						pgxmock.AnyArg(),   // external_id
						"John Doe",         // name
						"john@example.com", // email
						"+6281234567890",   // phone
						pgxmock.AnyArg(),   // payment_code
						pgtype.Timestamp{Time: expiredAt, Valid: true}, // expired_at
					).
					WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(int32(1)))

				s.Publisher.EXPECT().Publish(
					gomock.Any(),
					constant.SubjectIncrementCategoryQuantity,
					gomock.Any(),
				).Return(nil, nil).Times(2)

				s.Publisher.EXPECT().Publish(
					gomock.Any(),
					constant.SubjectCreateOrder,
					gomock.Any(),
				).Return(nil, fmt.Errorf("publish error"))
			},
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   `{"error":"Internal Server Error"}`,
			timeNow: func() time.Time {
				return time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
			},
		},
		{
			name:    "success",
			reqBody: `{"category_id": 1, "name": "John Doe", "email": "john@example.com", "phone": "+6281234567890"}`,
			setupMock: func() {
				s.CacheMock.ExpectSetNX(fmt.Sprintf(constant.OrderEmailLock, "john@example.com"), true, constant.OrderEmailLockDefaultTTL).
					SetVal(true)
				s.PgxMock.ExpectQuery(`SELECT EXISTS \(SELECT 1 FROM orders WHERE email = \$1 AND status = 'pending'\) AS "exists"`).
					WithArgs("john@example.com").
					WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))
				s.CacheMock.ExpectDecr(fmt.Sprintf(constant.EachCategoryQuantityKey, int16(1))).
					SetVal(0)

				fixedTime := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
				expiredAt := fixedTime.Add(15 * time.Minute)

				s.PgxMock.ExpectQuery("INSERT INTO orders").
					WithArgs(
						int16(1),           // category_id
						pgxmock.AnyArg(),   // external_id
						"John Doe",         // name
						"john@example.com", // email
						"+6281234567890",   // phone
						pgxmock.AnyArg(),   // payment_code
						pgtype.Timestamp{Time: expiredAt, Valid: true}, // expired_at
					).
					WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(int32(1)))

				s.Publisher.EXPECT().Publish(
					gomock.Any(),
					constant.SubjectIncrementCategoryQuantity,
					gomock.Any(),
				).Return(nil, nil)

				s.Publisher.EXPECT().Publish(
					gomock.Any(),
					constant.SubjectCreateOrder,
					gomock.Any(),
				).Return(nil, nil)
			},
			expectedStatus: http.StatusOK,
			timeNow: func() time.Time {
				return time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
			},
		},
	}

	for _, tc := range tests {
		s.Run(tc.name, func() {
			orderHttp := RegisterOrderHttp(
				http.NewServeMux(),
				s.Cfg,
				s.Querier,
				s.Cache,
				s.Publisher,
				s.Validate,
				message.NewPrinter(language.Indonesian),
			)

			if tc.timeNow != nil {
				orderHttp.TimeNow = tc.timeNow
			}

			tc.setupMock()

			req := httptest.NewRequest(http.MethodPost, "/api/orders", strings.NewReader(tc.reqBody))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			orderHttp.create(w, req)

			s.Equal(tc.expectedStatus, w.Code)

			if tc.expectedStatus == http.StatusOK {
				s.Contains(w.Body.String(), tc.expectedBody, "Response should contain expected text")
			} else {
				actual := strings.TrimSpace(w.Body.String())
				s.Equal(tc.expectedBody, actual)
			}

			s.NoError(s.CacheMock.ExpectationsWereMet())
			s.NoError(s.PgxMock.ExpectationsWereMet())
		})
	}
}

func (s *OrderHttpTestSuite) TestCancel() {
	tests := []struct {
		name           string
		setupMock      func()
		expectedStatus int
		expectedBody   string
	}{
		{
			name: "database error",
			setupMock: func() {
				s.PgxMock.ExpectQuery(`UPDATE orders SET status = 'cancelled', updated_at = NOW\(\) WHERE id IN \(SELECT id FROM orders WHERE status = 'pending' AND expired_at < NOW\(\) LIMIT \$1\) RETURNING id, category_id, name, email`).
					WithArgs(int32(10)).
					WillReturnError(fmt.Errorf("database error"))
			},
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   `{"error":"Internal Server Error"}`,
		},
		{
			name: "no cancelable orders",
			setupMock: func() {
				s.PgxMock.ExpectQuery(`UPDATE orders SET status = 'cancelled', updated_at = NOW\(\) WHERE id IN \(SELECT id FROM orders WHERE status = 'pending' AND expired_at < NOW\(\) LIMIT \$1\) RETURNING id, category_id, name, email`).
					WithArgs(int32(10)).
					WillReturnRows(pgxmock.NewRows([]string{"id", "category_id", "name", "email"}))
			},
			expectedStatus: http.StatusOK,
			expectedBody:   ``,
		},
		{
			name: "redis incrby error",
			setupMock: func() {
				rows := pgxmock.NewRows([]string{"id", "category_id", "name", "email"}).
					AddRow(int32(1), int16(1), "John Doe", "john@example.com")

				s.PgxMock.ExpectQuery(`UPDATE orders SET status = 'cancelled', updated_at = NOW\(\) WHERE id IN \(SELECT id FROM orders WHERE status = 'pending' AND expired_at < NOW\(\) LIMIT \$1\) RETURNING id, category_id, name, email`).
					WithArgs(int32(10)).
					WillReturnRows(rows)

				s.CacheMock.ExpectIncrBy(fmt.Sprintf(constant.EachCategoryQuantityKey, int16(1)), int64(1)).SetErr(redis.ErrClosed)
			},
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   `{"error":"Internal Server Error"}`,
		},
		{
			name: "publish increment category error",
			setupMock: func() {
				rows := pgxmock.NewRows([]string{"id", "category_id", "name", "email"}).
					AddRow(int32(1), int16(1), "John Doe", "john@example.com")

				s.PgxMock.ExpectQuery(`UPDATE orders SET status = 'cancelled', updated_at = NOW\(\) WHERE id IN \(SELECT id FROM orders WHERE status = 'pending' AND expired_at < NOW\(\) LIMIT \$1\) RETURNING id, category_id, name, email`).
					WithArgs(int32(10)).
					WillReturnRows(rows)

				s.CacheMock.ExpectIncrBy(fmt.Sprintf(constant.EachCategoryQuantityKey, int16(1)), int64(1)).SetVal(1)

				s.Publisher.EXPECT().Publish(
					gomock.Any(),
					constant.SubjectBulkIncrementCategoryQuantity,
					gomock.Any(),
				).Return(nil, fmt.Errorf("publish error"))
			},
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   `{"error":"Internal Server Error"}`,
		},
		{
			name: "publish email error",
			setupMock: func() {
				rows := pgxmock.NewRows([]string{"id", "category_id", "name", "email"}).
					AddRow(int32(1), int16(1), "John Doe", "john@example.com")

				s.PgxMock.ExpectQuery(`UPDATE orders SET status = 'cancelled', updated_at = NOW\(\) WHERE id IN \(SELECT id FROM orders WHERE status = 'pending' AND expired_at < NOW\(\) LIMIT \$1\) RETURNING id, category_id, name, email`).
					WithArgs(int32(10)).
					WillReturnRows(rows)

				s.CacheMock.ExpectIncrBy(fmt.Sprintf(constant.EachCategoryQuantityKey, int16(1)), int64(1)).SetVal(1)

				s.Publisher.EXPECT().Publish(
					gomock.Any(),
					constant.SubjectBulkIncrementCategoryQuantity,
					gomock.Any(),
				).Return(nil, nil)

				s.Publisher.EXPECT().Publish(
					gomock.Any(),
					constant.SubjectSendEmail,
					gomock.Any(),
				).Return(nil, fmt.Errorf("publish error"))
			},
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   `{"error":"Internal Server Error"}`,
		},
		{
			name: "success",
			setupMock: func() {
				rows := pgxmock.NewRows([]string{"id", "category_id", "name", "email"}).
					AddRow(int32(1), int16(1), "John Doe", "john@example.com")

				s.PgxMock.ExpectQuery(`UPDATE orders SET status = 'cancelled', updated_at = NOW\(\) WHERE id IN \(SELECT id FROM orders WHERE status = 'pending' AND expired_at < NOW\(\) LIMIT \$1\) RETURNING id, category_id, name, email`).
					WithArgs(int32(10)).
					WillReturnRows(rows)

				s.CacheMock.ExpectIncrBy(fmt.Sprintf(constant.EachCategoryQuantityKey, int16(1)), int64(1)).SetVal(1)

				// Expect successful category increment publishes
				s.Publisher.EXPECT().Publish(
					gomock.Any(),
					constant.SubjectBulkIncrementCategoryQuantity,
					gomock.Any(),
				).Return(nil, nil)

				// Expect successful email publishes
				s.Publisher.EXPECT().Publish(
					gomock.Any(),
					constant.SubjectSendEmail,
					gomock.Any(),
				).Return(nil, nil)
			},
			expectedStatus: http.StatusOK,
			expectedBody:   ``,
		},
	}

	for _, tc := range tests {
		s.Run(tc.name, func() {
			orderHttp := RegisterOrderHttp(
				http.NewServeMux(),
				s.Cfg,
				s.Querier,
				s.Cache,
				s.Publisher,
				s.Validate,
				message.NewPrinter(language.Indonesian),
			)

			tc.setupMock()

			req := httptest.NewRequest(http.MethodPost, "/api/orders/cancel", nil)
			w := httptest.NewRecorder()

			orderHttp.cancel(w, req)

			if tc.expectedStatus == http.StatusOK {
				s.Contains(w.Body.String(), tc.expectedBody, "Response should contain expected text")
			} else {
				actual := strings.TrimSpace(w.Body.String())
				s.Equal(tc.expectedBody, actual)
			}

			s.NoError(s.CacheMock.ExpectationsWereMet())
			s.NoError(s.PgxMock.ExpectationsWereMet())
		})
	}
}
