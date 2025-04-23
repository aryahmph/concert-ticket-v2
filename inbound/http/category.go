package http

import (
	"concert-ticket/common/vars"
	"concert-ticket/outbound/sqlgen"
	"github.com/redis/go-redis/v9"
	"net/http"
)

type CategoryHttp struct {
	Querier *sqlgen.Queries
	Cache   *redis.Client
}

func RegisterCategoryHttp(mux *http.ServeMux, querier *sqlgen.Queries, cache *redis.Client) *CategoryHttp {
	in := &CategoryHttp{Querier: querier, Cache: cache}

	mux.HandleFunc("GET /api/categories", in.list)

	return in
}

func (in *CategoryHttp) list(w http.ResponseWriter, r *http.Request) {
	writeJSONResponse(w, http.StatusOK, vars.GetCategories())
}
