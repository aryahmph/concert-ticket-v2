package http

import (
	"concert-ticket/common/vars"
	"concert-ticket/model"
	"concert-ticket/outbound/sqlgen"
	"github.com/go-redis/redismock/v9"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/suite"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type CategoryHttpTestSuite struct {
	suite.Suite

	Querier *sqlgen.Queries
	PgxMock pgxmock.PgxPoolIface

	Cache     *redis.Client
	CacheMock redismock.ClientMock

	Categories []model.CategoryResponse
}

func (s *CategoryHttpTestSuite) SetupTest() {
	rdb, mock := redismock.NewClientMock()
	s.Cache = rdb
	s.CacheMock = mock

	pool, err := pgxmock.NewPool()
	if err != nil {
		s.T().Fatalf("failed to create pgxmock pool: %v", err)
	}

	s.PgxMock = pool
	s.Querier = sqlgen.New(pool)

	slog.SetLogLoggerLevel(slog.LevelDebug)
}

func (s *CategoryHttpTestSuite) TearDownTest() {
	s.PgxMock.Close()

	if err := s.Cache.Close(); err != nil {
		s.T().Fatalf("failed to close redis mock: %v", err)
	}
}

func TestCategoryHttpTestSuite(t *testing.T) {
	suite.Run(t, new(CategoryHttpTestSuite))
}

func (s *CategoryHttpTestSuite) TestList() {
	tests := []struct {
		name           string
		setupVars      func()
		expectedStatus int
		expectedBody   string
	}{
		{
			name: "success with categories",
			setupVars: func() {
				categories := []model.CategoryResponse{
					{
						Id:       1,
						Name:     "Category 1",
						Price:    100,
						Quantity: 10,
					},
				}
				vars.SetCategories(categories)
			},
			expectedStatus: http.StatusOK,
			expectedBody:   `[{"id":1,"name":"Category 1","price":100,"quantity":10}]`,
		},
		{
			name: "success with empty categories",
			setupVars: func() {
				vars.SetCategories([]model.CategoryResponse{})
			},
			expectedStatus: http.StatusOK,
			expectedBody:   `null`,
		},
	}

	for _, tc := range tests {
		s.Run(tc.name, func() {
			// Setup test data in vars package
			tc.setupVars()

			categoryHttp := RegisterCategoryHttp(http.NewServeMux(), s.Querier, s.Cache)

			req := httptest.NewRequest(http.MethodGet, "/api/categories", nil)
			w := httptest.NewRecorder()

			categoryHttp.list(w, req)

			s.Equal(tc.expectedStatus, w.Code)

			actual := strings.TrimSpace(w.Body.String())
			s.Equal(tc.expectedBody, actual)
		})
	}
}
