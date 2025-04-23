package event

import (
	"concert-ticket/model"
	"concert-ticket/outbound/sqlgen"
	"context"
	"encoding/json"
	"fmt"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/suite"
	"log/slog"
	"testing"
	"time"
)

type CategoryEventTestSuite struct {
	suite.Suite
	PgxMock       pgxmock.PgxPoolIface
	categoryEvent CategoryEvent
}

func (s *CategoryEventTestSuite) SetupTest() {
	pool, err := pgxmock.NewPool()
	if err != nil {
		s.T().Fatalf("failed to create pgxmock pool: %v", err)
	}

	s.PgxMock = pool
	s.categoryEvent = CategoryEvent{
		Querier: sqlgen.New(pool),
		Timeout: 10 * time.Second,
	}

	slog.SetLogLoggerLevel(slog.LevelDebug)
}

func (s *CategoryEventTestSuite) TearDownTest() {
	s.PgxMock.Close()
}

func TestCategoryEventTestSuite(t *testing.T) {
	suite.Run(t, new(CategoryEventTestSuite))
}

func (s *CategoryEventTestSuite) TestBulkIncrementCategoryQuantityHandler() {
	testCases := []struct {
		name        string
		input       []model.IncrementCategoryQuantityEventMessage
		setupMock   func()
		expectError bool
	}{
		{
			name:  "invalid json",
			input: []model.IncrementCategoryQuantityEventMessage{
				// This will be ignored as we'll send invalid JSON
			},
			setupMock:   func() {},
			expectError: false,
		},
		{
			name: "database error",
			input: []model.IncrementCategoryQuantityEventMessage{
				{ID: 1, Quantity: 5},
				{ID: 2, Quantity: 10},
			},
			setupMock: func() {
				s.PgxMock.ExpectExec("UPDATE categories").
					WithArgs(int32(5), int32(10), int32(0), int32(0), int32(0), int32(0), int32(0), int32(0), int32(0)).
					WillReturnError(fmt.Errorf("database error"))
			},
			expectError: true,
		},
		{
			name: "success",
			input: []model.IncrementCategoryQuantityEventMessage{
				{ID: 1, Quantity: 5},
				{ID: 3, Quantity: 15},
				{ID: 9, Quantity: 25},
				{ID: 3, Quantity: 10}, // Testing accumulation for same category
			},
			setupMock: func() {
				s.PgxMock.ExpectExec("UPDATE categories").
					WithArgs(int32(5), int32(0), int32(25), int32(0), int32(0), int32(0), int32(0), int32(0), int32(25)).
					WillReturnResult(pgxmock.NewResult("UPDATE", 9))
			},
			expectError: false,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			tc.setupMock()

			var msg []byte
			var err error

			if tc.name == "invalid json" {
				msg = []byte(`{"invalid": "json"}`)
			} else {
				msg, err = json.Marshal(tc.input)
				s.Require().NoError(err)
			}

			err = s.categoryEvent.BulkIncrementCategoryQuantityHandler(context.Background(), msg)

			if tc.expectError {
				s.Error(err)
			} else {
				s.NoError(err)
			}

			s.NoError(s.PgxMock.ExpectationsWereMet())
		})
	}
}

func (s *CategoryEventTestSuite) TestIncrementCategoryQuantityHandler() {
	testCases := []struct {
		name        string
		input       model.IncrementCategoryQuantityEventMessage
		expectedOut model.IncrementCategoryQuantityEventMessage
	}{
		{
			name:        "invalid json",
			input:       model.IncrementCategoryQuantityEventMessage{},
			expectedOut: model.IncrementCategoryQuantityEventMessage{},
		},
		{
			name: "valid message",
			input: model.IncrementCategoryQuantityEventMessage{
				ID:       3,
				Quantity: 15,
			},
			expectedOut: model.IncrementCategoryQuantityEventMessage{
				ID:       3,
				Quantity: 15,
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			var msg []byte
			var err error

			if tc.name == "invalid json" {
				msg = []byte(`{"invalid": "json"}`)
			} else {
				msg, err = json.Marshal(tc.input)
				s.Require().NoError(err)
			}

			result := s.categoryEvent.IncrementCategoryQuantityHandler(context.Background(), msg)

			if tc.name == "invalid json" {
				s.Equal(tc.expectedOut, result)
			} else {
				s.Equal(tc.input.ID, result.ID)
				s.Equal(tc.input.Quantity, result.Quantity)
			}
		})
	}
}
