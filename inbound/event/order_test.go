package event

import (
	"concert-ticket/common/constant"
	jetsteamMock "concert-ticket/common/jetstream/mocks"
	"concert-ticket/model"
	"concert-ticket/outbound/sqlgen"
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"log/slog"
	"testing"
	"time"
)

type OrderEventTestSuite struct {
	suite.Suite
	ctrl       *gomock.Controller
	publisher  *jetsteamMock.MockPublisher
	Querier    *sqlgen.Queries
	PgxMock    pgxmock.PgxPoolIface
	orderEvent OrderEvent
}

func (s *OrderEventTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.publisher = jetsteamMock.NewMockPublisher(s.ctrl)

	idrPrinter := message.NewPrinter(language.Indonesian)
	s.orderEvent = OrderEvent{
		Publisher:            s.publisher,
		IdrCurrencyFormatter: idrPrinter,
	}

	pool, err := pgxmock.NewPool()
	if err != nil {
		s.T().Fatalf("failed to create pgxmock pool: %v", err)
	}

	s.orderEvent.Db = pool
	s.PgxMock = pool
	s.Querier = sqlgen.New(pool)

	s.orderEvent.Timeout = 10 * time.Second
	slog.SetLogLoggerLevel(slog.LevelDebug)
}

func (s *OrderEventTestSuite) TearDownTest() {
	s.PgxMock.Close()
	s.ctrl.Finish()
}

func TestOrderEventTestSuite(t *testing.T) {
	suite.Run(t, new(OrderEventTestSuite))
}

func (s *OrderEventTestSuite) TestCreate() {
	testCases := []struct {
		name        string
		input       model.CreateOrderEventMessage
		setupMock   func(msg []byte)
		expectError bool
	}{
		{
			name: "publish error",
			input: model.CreateOrderEventMessage{
				ID:          123,
				Email:       "john@example.com",
				Name:        "John Doe",
				CategoryID:  1,
				PaymentCode: "PAYMENT123",
				ExpiredAt:   time.Now().Add(15 * time.Minute).Format(time.DateTime),
			},
			setupMock: func(msg []byte) {
				s.publisher.EXPECT().Publish(
					gomock.Any(),
					constant.SubjectSendEmail,
					gomock.Any(),
				).Return(nil, fmt.Errorf("publish error"))
			},
			expectError: true,
		},
		{
			name: "success",
			input: model.CreateOrderEventMessage{
				ID:          123,
				Email:       "john@example.com",
				Name:        "John Doe",
				CategoryID:  1,
				PaymentCode: "PAYMENT123",
				ExpiredAt:   time.Now().Add(15 * time.Minute).Format(time.DateTime),
			},
			setupMock: func(msg []byte) {

				s.publisher.EXPECT().Publish(
					gomock.Any(),
					constant.SubjectSendEmail,
					gomock.Any(),
				).Return(nil, nil)
			},
			expectError: false,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			var msg []byte
			var err error

			msg, err = json.Marshal(tc.input)
			s.Require().NoError(err)

			tc.setupMock(msg)
			err = s.orderEvent.CreateHandler(context.Background(), msg)

			if tc.expectError {
				s.Error(err)
			} else {
				s.NoError(err)
			}
		})
	}
}

func (s *OrderEventTestSuite) TestComplete() {
	fixedTime := time.Now()

	testCases := []struct {
		name        string
		input       model.PaymentCallbackRequest
		setupMock   func(msg []byte)
		expectError bool
	}{
		{
			name: "find order error",
			input: model.PaymentCallbackRequest{
				ExternalId: "order-123",
			},
			setupMock: func(msg []byte) {
				s.PgxMock.ExpectQuery("SELECT (.+) FROM orders").
					WithArgs("order-123").
					WillReturnError(fmt.Errorf("database error"))
			},
			expectError: true,
		},
		{
			name: "order not found",
			input: model.PaymentCallbackRequest{
				ExternalId: "order-123",
			},
			setupMock: func(msg []byte) {
				s.PgxMock.ExpectQuery("SELECT (.+) FROM orders").
					WithArgs("order-123").
					WillReturnError(pgx.ErrNoRows)
			},
			expectError: false,
		},
		{
			name: "update order status error",
			input: model.PaymentCallbackRequest{
				ExternalId: "order-123",
			},
			setupMock: func(msg []byte) {
				rows := pgxmock.NewRows([]string{"id", "category_id", "external_id", "name", "email", "payment_code", "expired_at"})
				rows.AddRow(int32(1), int16(1), "order-123", "John Doe", "john@example.com", "PAY123", fixedTime)

				s.PgxMock.ExpectQuery("SELECT (.+) FROM orders").
					WithArgs("order-123").
					WillReturnRows(rows)

				s.PgxMock.ExpectExec("UPDATE orders SET (.+)").
					WithArgs(int32(1)).
					WillReturnError(fmt.Errorf("update error"))
			},
			expectError: true,
		},
		{
			name: "no rows affected",
			input: model.PaymentCallbackRequest{
				ExternalId: "order-123",
			},
			setupMock: func(msg []byte) {
				rows := pgxmock.NewRows([]string{"id", "category_id", "external_id", "name", "email", "payment_code", "expired_at"})
				rows.AddRow(int32(1), int16(1), "order-123", "John Doe", "john@example.com", "PAY123", fixedTime)

				s.PgxMock.ExpectQuery("SELECT (.+) FROM orders").
					WithArgs("order-123").
					WillReturnRows(rows)

				s.PgxMock.ExpectExec("UPDATE orders SET (.+)").
					WithArgs(int32(1)).
					WillReturnResult(pgxmock.NewResult("UPDATE", 0))
			},
			expectError: false,
		},
		{
			name: "publish error",
			input: model.PaymentCallbackRequest{
				ExternalId: "order-123",
			},
			setupMock: func(msg []byte) {
				rows := pgxmock.NewRows([]string{"id", "category_id", "external_id", "name", "email", "payment_code", "expired_at"})
				rows.AddRow(int32(1), int16(1), "order-123", "John Doe", "john@example.com", "PAY123", fixedTime)

				s.PgxMock.ExpectQuery("SELECT (.+) FROM orders").
					WithArgs("order-123").
					WillReturnRows(rows)

				s.PgxMock.ExpectExec("UPDATE orders SET (.+)").
					WithArgs(int32(1)).
					WillReturnResult(pgxmock.NewResult("UPDATE", 1))

				s.publisher.EXPECT().Publish(
					gomock.Any(),
					constant.SubjectAssignOrderTicketRowCol,
					gomock.Any(),
				).Return(nil, fmt.Errorf("publish error"))
			},
			expectError: true,
		},
		{
			name: "success",
			input: model.PaymentCallbackRequest{
				ExternalId: "order-123",
			},
			setupMock: func(msg []byte) {
				rows := pgxmock.NewRows([]string{"id", "category_id", "external_id", "name", "email", "payment_code", "expired_at"})
				rows.AddRow(int32(1), int16(1), "order-123", "John Doe", "john@example.com", "PAY123", fixedTime)

				s.PgxMock.ExpectQuery("SELECT (.+) FROM orders").
					WithArgs("order-123").
					WillReturnRows(rows)

				s.PgxMock.ExpectExec("UPDATE orders SET (.+)").
					WithArgs(int32(1)).
					WillReturnResult(pgxmock.NewResult("UPDATE", 1))

				s.publisher.EXPECT().Publish(
					gomock.Any(),
					constant.SubjectAssignOrderTicketRowCol,
					gomock.Any(),
				).Return(nil, nil)
			},
			expectError: false,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			var msg []byte
			var err error

			msg, err = json.Marshal(tc.input)
			s.Require().NoError(err)

			s.orderEvent.Querier = sqlgen.New(s.PgxMock)
			tc.setupMock(msg)
			err = s.orderEvent.CompleteHandler(context.Background(), msg)

			if tc.expectError {
				s.Error(err)
			} else {
				s.NoError(err)
			}

			s.NoError(s.PgxMock.ExpectationsWereMet())
		})
	}
}

func (s *OrderEventTestSuite) TestAssignTicketCol() {
	testCases := []struct {
		name        string
		input       model.AssignOrderTicketRowCol
		setupMock   func(msg []byte)
		expectError bool
	}{
		{
			name: "transaction begin error",
			input: model.AssignOrderTicketRowCol{
				ID:         1,
				CategoryId: 1,
				Email:      "john@example.com",
				Name:       "John Doe",
			},
			setupMock: func(msg []byte) {
				s.PgxMock.ExpectBegin().WillReturnError(fmt.Errorf("begin error"))
			},
			expectError: true,
		},
		{
			name: "decrement category quantity error",
			input: model.AssignOrderTicketRowCol{
				ID:         1,
				CategoryId: 1,
				Email:      "john@example.com",
				Name:       "John Doe",
			},
			setupMock: func(msg []byte) {
				s.PgxMock.ExpectBegin()
				s.PgxMock.ExpectQuery("WITH selected_quantity AS").
					WithArgs(int16(1)).
					WillReturnError(fmt.Errorf("decrement error"))
				s.PgxMock.ExpectRollback().WillReturnError(nil)
			},
			expectError: true,
		},
		{
			name: "category quantity row is 0",
			input: model.AssignOrderTicketRowCol{
				ID:         1,
				CategoryId: 1,
				Email:      "john@example.com",
				Name:       "John Doe",
			},
			setupMock: func(msg []byte) {
				s.PgxMock.ExpectBegin()
				rows := pgxmock.NewRows([]string{"row", "col"}).
					AddRow(int32(0), int32(5))
				s.PgxMock.ExpectQuery("WITH selected_quantity AS").
					WithArgs(int16(1)).
					WillReturnRows(rows)
				s.PgxMock.ExpectRollback().WillReturnError(nil)
			},
			expectError: true,
		},
		{
			name: "category quantity col is negative",
			input: model.AssignOrderTicketRowCol{
				ID:         1,
				CategoryId: 1,
				Email:      "john@example.com",
				Name:       "John Doe",
			},
			setupMock: func(msg []byte) {
				s.PgxMock.ExpectBegin()
				rows := pgxmock.NewRows([]string{"row", "col"}).
					AddRow(int32(1), int32(-1))
				s.PgxMock.ExpectQuery("WITH selected_quantity AS").
					WithArgs(int16(1)).
					WillReturnRows(rows)
				s.PgxMock.ExpectRollback().WillReturnError(nil)
			},
			expectError: true,
		},
		{
			name: "update order ticket row col error",
			input: model.AssignOrderTicketRowCol{
				ID:         1,
				CategoryId: 1,
				Email:      "john@example.com",
				Name:       "John Doe",
			},
			setupMock: func(msg []byte) {
				s.PgxMock.ExpectBegin()
				rows := pgxmock.NewRows([]string{"row", "col"}).
					AddRow(int32(1), int32(5))
				s.PgxMock.ExpectQuery("WITH selected_quantity AS").
					WithArgs(int16(1)).
					WillReturnRows(rows)
				s.PgxMock.ExpectExec("UPDATE orders SET (.+)").
					WithArgs(pgtype.Int4{Int32: int32(1), Valid: true}, pgtype.Int4{Int32: int32(5), Valid: true}, int32(1)).
					WillReturnError(fmt.Errorf("update error"))
				s.PgxMock.ExpectRollback().WillReturnError(nil)
			},
			expectError: true,
		},
		{
			name: "commit transaction error",
			input: model.AssignOrderTicketRowCol{
				ID:         1,
				CategoryId: 1,
				Email:      "john@example.com",
				Name:       "John Doe",
			},
			setupMock: func(msg []byte) {
				s.PgxMock.ExpectBegin()
				rows := pgxmock.NewRows([]string{"row", "col"}).
					AddRow(int32(1), int32(5))
				s.PgxMock.ExpectQuery("WITH selected_quantity AS").
					WithArgs(int16(1)).
					WillReturnRows(rows)
				s.PgxMock.ExpectExec("UPDATE orders SET (.+)").
					WithArgs(pgtype.Int4{Int32: int32(1), Valid: true}, pgtype.Int4{Int32: int32(5), Valid: true}, int32(1)).
					WillReturnResult(pgxmock.NewResult("UPDATE", 1))
				s.PgxMock.ExpectCommit().WillReturnError(fmt.Errorf("commit error"))
				s.PgxMock.ExpectRollback().WillReturnError(nil)
			},
			expectError: true,
		},
		{
			name: "publish message error",
			input: model.AssignOrderTicketRowCol{
				ID:         1,
				CategoryId: 1,
				Email:      "john@example.com",
				Name:       "John Doe",
			},
			setupMock: func(msg []byte) {
				s.PgxMock.ExpectBegin()
				rows := pgxmock.NewRows([]string{"row", "col"}).
					AddRow(int32(1), int32(5))
				s.PgxMock.ExpectQuery("WITH selected_quantity AS").
					WithArgs(int16(1)).
					WillReturnRows(rows)
				s.PgxMock.ExpectExec("UPDATE orders SET (.+)").
					WithArgs(pgtype.Int4{Int32: int32(1), Valid: true}, pgtype.Int4{Int32: int32(5), Valid: true}, int32(1)).
					WillReturnResult(pgxmock.NewResult("UPDATE", 1))
				s.PgxMock.ExpectCommit().WillReturnError(nil)

				s.publisher.EXPECT().Publish(
					gomock.Any(),
					constant.SubjectSendEmail,
					gomock.Any(),
				).Return(nil, fmt.Errorf("publish error"))
			},
			expectError: true,
		},
		{
			name: "success",
			input: model.AssignOrderTicketRowCol{
				ID:         1,
				CategoryId: 1,
				Email:      "john@example.com",
				Name:       "John Doe",
			},
			setupMock: func(msg []byte) {
				s.PgxMock.ExpectBegin()
				rows := pgxmock.NewRows([]string{"row", "col"}).
					AddRow(int32(1), int32(5))
				s.PgxMock.ExpectQuery("WITH selected_quantity AS").
					WithArgs(int16(1)).
					WillReturnRows(rows)
				s.PgxMock.ExpectExec("UPDATE orders SET (.+)").
					WithArgs(pgtype.Int4{Int32: int32(1), Valid: true}, pgtype.Int4{Int32: int32(5), Valid: true}, int32(1)).
					WillReturnResult(pgxmock.NewResult("UPDATE", 1))
				s.PgxMock.ExpectCommit().WillReturnError(nil)

				s.publisher.EXPECT().Publish(
					gomock.Any(),
					constant.SubjectSendEmail,
					gomock.Any(),
				).Return(nil, nil)
			},
			expectError: false,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			var msg []byte
			var err error

			msg, err = json.Marshal(tc.input)
			s.Require().NoError(err)

			s.orderEvent.Db = s.PgxMock
			s.orderEvent.Querier = sqlgen.New(s.PgxMock)
			tc.setupMock(msg)
			err = s.orderEvent.AssignTicketColHandler(context.Background(), msg)

			if tc.expectError {
				s.Error(err)
			} else {
				s.NoError(err)
			}

			s.NoError(s.PgxMock.ExpectationsWereMet())
		})
	}
}
