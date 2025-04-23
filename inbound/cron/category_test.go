package cron

import (
	"concert-ticket/common/constant"
	"concert-ticket/common/vars"
	"concert-ticket/model"
	"concert-ticket/outbound/sqlgen"
	"context"
	"fmt"
	"github.com/go-redis/redismock/v9"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/suite"
	"log/slog"
	"testing"
	"time"
)

type CategoryCronTestSuite struct {
	suite.Suite

	Querier *sqlgen.Queries
	PgxMock pgxmock.PgxPoolIface

	Cache     *redis.Client
	CacheMock redismock.ClientMock

	Cfg *viper.Viper
}

func (s *CategoryCronTestSuite) SetupTest() {
	pool, err := pgxmock.NewPool()
	if err != nil {
		s.T().Fatalf("failed to create pgxmock pool: %v", err)
	}

	s.PgxMock = pool
	s.Querier = sqlgen.New(pool)

	rdb, mock := redismock.NewClientMock()
	s.Cache = rdb
	s.CacheMock = mock

	s.Cfg = viper.New()
	s.Cfg.Set("cron.category.refresh.interval", "5s")
	s.Cfg.Set("cron.category.refresh.timeout", "10s")

	// Initialize test data
	constant.CategoriesData = []model.CategoryResponse{
		{
			Id:       1,
			Name:     "Category 1",
			Price:    100,
			Quantity: 0,
		},
		{
			Id:       2,
			Name:     "Category 2",
			Price:    200,
			Quantity: 0,
		},
	}

	slog.SetLogLoggerLevel(slog.LevelDebug)
}

func (s *CategoryCronTestSuite) TearDownTest() {
	s.PgxMock.Close()

	if err := s.Cache.Close(); err != nil {
		s.T().Fatalf("failed to close redis mock: %v", err)
	}

	// Reset the categories
	vars.SetCategories(nil)
}

func TestCategoryCronTestSuite(t *testing.T) {
	suite.Run(t, new(CategoryCronTestSuite))
}

func (s *CategoryCronTestSuite) TestRefresh() {
	tests := []struct {
		name           string
		setupMock      func()
		expectedResult []model.CategoryResponse
	}{
		{
			name: "cache error",
			setupMock: func() {
				s.CacheMock.ExpectMGet("category:1:quantity", "category:2:quantity").
					SetErr(redis.ErrClosed)
			},
			expectedResult: nil,
		},
		{
			name: "success with zero quantities",
			setupMock: func() {
				s.CacheMock.ExpectMGet("category:1:quantity", "category:2:quantity").
					SetVal([]interface{}{"", ""})
			},
			expectedResult: []model.CategoryResponse{
				{
					Id:       1,
					Name:     "Category 1",
					Price:    100,
					Quantity: 0,
				},
				{
					Id:       2,
					Name:     "Category 2",
					Price:    200,
					Quantity: 0,
				},
			},
		},
		{
			name: "success with actual quantities",
			setupMock: func() {
				s.CacheMock.ExpectMGet("category:1:quantity", "category:2:quantity").
					SetVal([]interface{}{"50", "75"})
			},
			expectedResult: []model.CategoryResponse{
				{
					Id:       1,
					Name:     "Category 1",
					Price:    100,
					Quantity: 50,
				},
				{
					Id:       2,
					Name:     "Category 2",
					Price:    200,
					Quantity: 75,
				},
			},
		},
		{
			name: "invalid quantity value",
			setupMock: func() {
				s.CacheMock.ExpectMGet("category:1:quantity", "category:2:quantity").
					SetVal([]interface{}{"not-a-number", "75"})
			},
			expectedResult: nil,
		},
	}

	for _, tc := range tests {
		s.Run(tc.name, func() {
			// Reset categories before each test
			vars.SetCategories(nil)

			categoryCron := CategoryCron{
				Cfg:     s.Cfg,
				Cache:   s.Cache,
				Querier: s.Querier,
			}

			tc.setupMock()

			ctx := context.Background()
			categoryCron.refresh(ctx)

			if tc.expectedResult == nil {
				s.Nil(vars.GetCategories())
			} else {
				s.Equal(tc.expectedResult, vars.GetCategories())
			}

			s.NoError(s.CacheMock.ExpectationsWereMet())
		})
	}
}

func (s *CategoryCronTestSuite) TestStart() {
	// Setup mock for initial refresh
	s.CacheMock.ExpectMGet("category:1:quantity", "category:2:quantity").
		SetVal([]interface{}{"50", "75"})

	// Set a shorter refresh interval for testing
	s.Cfg.Set("cron.category.refresh.interval", "200ms")

	categoryCron := CategoryCron{
		Cfg:     s.Cfg,
		Cache:   s.Cache,
		Querier: s.Querier,
	}

	// Create a context with cancel to stop the cron
	ctx, cancel := context.WithCancel(context.Background())

	// Run the cron in a goroutine since it blocks
	go func() {
		categoryCron.Start(ctx)
	}()

	// Wait a bit to ensure the initial refresh completes
	time.Sleep(100 * time.Millisecond)

	// Verify that categories were updated in the initial refresh
	expected := []model.CategoryResponse{
		{
			Id:       1,
			Name:     "Category 1",
			Price:    100,
			Quantity: 50,
		},
		{
			Id:       2,
			Name:     "Category 2",
			Price:    200,
			Quantity: 75,
		},
	}
	s.Equal(expected, vars.GetCategories())

	// Setup mock for the next refresh cycle
	s.CacheMock.ExpectMGet("category:1:quantity", "category:2:quantity").
		SetVal([]interface{}{"60", "85"})

	// Wait for the next refresh cycle
	time.Sleep(250 * time.Millisecond)

	// Verify categories were updated again
	updated := []model.CategoryResponse{
		{
			Id:       1,
			Name:     "Category 1",
			Price:    100,
			Quantity: 60,
		},
		{
			Id:       2,
			Name:     "Category 2",
			Price:    200,
			Quantity: 85,
		},
	}
	s.Equal(updated, vars.GetCategories())

	// Cancel the context to stop the cron
	cancel()

	// Give time for the goroutine to exit
	time.Sleep(100 * time.Millisecond)

	s.NoError(s.CacheMock.ExpectationsWereMet())
}

func (s *CategoryCronTestSuite) TestInitQuantityCache() {
	tests := []struct {
		name      string
		setupMock func()
		wantErr   bool
	}{
		{
			name: "database error",
			setupMock: func() {
				s.PgxMock.ExpectQuery("SELECT id, name, price, max_row, max_col, quantity FROM categories").
					WillReturnError(fmt.Errorf("database error"))
			},
			wantErr: true,
		},
		{
			name: "no categories found",
			setupMock: func() {
				s.PgxMock.ExpectQuery("SELECT id, name, price, max_row, max_col, quantity FROM categories").
					WillReturnRows(pgxmock.NewRows([]string{"id", "name", "price", "max_row", "max_col", "quantity"}))
			},
			wantErr: false,
		},
		{
			name: "redis pipeline error",
			setupMock: func() {
				rows := pgxmock.NewRows([]string{"id", "name", "price", "max_row", "max_col", "quantity"}).
					AddRow(int16(1), "Category 1", 100.0, int16(10), int16(10), int32(50)).
					AddRow(int16(2), "Category 2", 200.0, int16(5), int16(5), int32(75))

				s.PgxMock.ExpectQuery("SELECT id, name, price, max_row, max_col, quantity FROM categories").
					WillReturnRows(rows)

				s.CacheMock.ExpectTxPipeline()
				s.CacheMock.ExpectSetNX("category:1:quantity", int32(50), 0).SetVal(true)
				s.CacheMock.ExpectSetNX("category:2:quantity", int32(75), 0).SetVal(true)
				s.CacheMock.ExpectTxPipelineExec().SetErr(redis.ErrClosed)
			},
			wantErr: true,
		},
		{
			name: "success",
			setupMock: func() {
				rows := pgxmock.NewRows([]string{"id", "name", "price", "max_row", "max_col", "quantity"}).
					AddRow(int16(1), "Category 1", 100.0, int16(10), int16(10), int32(50)).
					AddRow(int16(2), "Category 2", 200.0, int16(5), int16(5), int32(75))

				s.PgxMock.ExpectQuery("SELECT id, name, price, max_row, max_col, quantity FROM categories").
					WillReturnRows(rows)

				s.CacheMock.ExpectTxPipeline()
				s.CacheMock.ExpectSetNX("category:1:quantity", int32(50), 0).SetVal(true)
				s.CacheMock.ExpectSetNX("category:2:quantity", int32(75), 0).SetVal(true)
				s.CacheMock.ExpectTxPipelineExec()
			},
			wantErr: false,
		},
	}

	for _, tc := range tests {
		s.Run(tc.name, func() {
			categoryCron := CategoryCron{
				Cfg:     s.Cfg,
				Cache:   s.Cache,
				Querier: s.Querier,
			}

			tc.setupMock()

			ctx := context.Background()
			err := categoryCron.InitQuantityCache(ctx)

			if tc.wantErr {
				s.Error(err)
			} else {
				s.NoError(err)
			}

			s.NoError(s.CacheMock.ExpectationsWereMet())
			s.NoError(s.PgxMock.ExpectationsWereMet())
		})
	}
}
