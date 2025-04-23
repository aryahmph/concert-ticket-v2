package cmd

import (
	inboundCron "concert-ticket/inbound/cron"
	inboundHttp "concert-ticket/inbound/http"
	"concert-ticket/outbound/sqlgen"
	"context"
	"fmt"
	"github.com/go-playground/validator/v10"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"log"
	"log/slog"
	"net/http"
	"os"
	"runtime/pprof"
	"time"
)

func runHttpServerCmd(ctx context.Context) {
	cfg := newCfg("env")

	if cfg.GetString("env") == "dev" {
		cpu, err := os.Create("http-cpu.prof")
		if err != nil {
			log.Fatalf("could not create CPU profile: %v", err)
		}
		defer cpu.Close()

		err = pprof.StartCPUProfile(cpu)
		if err != nil {
			log.Fatalf("could not start CPU profile: %v", err)
		}
		defer pprof.StopCPUProfile()

		mem, err := os.Create("http-mem.prof")
		if err != nil {
			log.Fatalf("could not create memory profile: %v", err)
		}
		defer mem.Close()

		err = pprof.WriteHeapProfile(mem)
		if err != nil {
			log.Fatalf("could not write memory profile: %v", err)
		}
		defer mem.Close()
	}

	validate := validator.New()

	db := newDb(cfg)
	defer db.Close()

	cacheClient := newRedis(cfg)
	defer cacheClient.Close()

	natsConn := newNats(cfg)
	defer natsConn.Close()

	js := newJs(natsConn)
	createStreamWorkQueue(ctx, js)

	querier := sqlgen.New(db)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		slog.DebugContext(r.Context(), "health check")
		w.WriteHeader(http.StatusOK)
	})

	timeoutMiddleware := inboundHttp.TimeoutMiddleware(20 * time.Second)

	inboundHttp.RegisterCategoryHttp(mux, querier, cacheClient)
	inboundHttp.RegisterOrderHttp(mux, cfg, querier, cacheClient, js, validate, message.NewPrinter(language.Indonesian))
	inboundHttp.RegisterPaymentHttp(mux, js, validate)

	categoryCron := &inboundCron.CategoryCron{
		Cfg:     cfg,
		Cache:   cacheClient,
		Querier: querier,
	}

	err := categoryCron.InitQuantityCache(ctx)
	if err != nil {
		log.Fatalln("unable to init category cache", err)
	}

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.GetInt("server.port")),
		Handler:           timeoutMiddleware(inboundHttp.CorsMiddleware(mux)),
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       120 * time.Second,
		ReadHeaderTimeout: 2 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalln("unable to start server", err)
		}
	}()

	slog.Info("http server started")

	go func() {
		categoryCron.Start(ctx)
	}()

	<-ctx.Done()

	ctxShutDown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctxShutDown); err != nil {
		log.Fatalln("unable to shutdown server", err)
	}

	slog.Info("http server stopped")
}
