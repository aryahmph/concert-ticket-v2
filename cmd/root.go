package cmd

import (
	"concert-ticket/common/otel"
	"context"
	"fmt"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/otel/sdk/resource"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"runtime/pprof"
	"time"
)

func Start() {
	cfg := newCfg("env")
	slog.SetLogLoggerLevel(slog.Level(cfg.GetInt("log.level")))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	var shutdownTracerProvider func(context.Context) error
	if cfg.GetString("env") == "dev" {
		// Setup tracing
		conn, err := grpc.NewClient("localhost:4317", grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			log.Fatalf("failed to create gRPC connection: %v", err)
		}
		defer conn.Close()

		res, err := resource.New(ctx, resource.WithAttributes(otel.ServiceName))
		if err != nil {
			log.Fatal(err)
		}

		shutdownTracerProvider, err = otel.InitTracerProvider(ctx, res, conn)
		if err != nil {
			log.Fatal(err)
		}
		defer shutdownTracerProvider(ctx)
	}

	rootCmd := &cobra.Command{}
	cmd := []*cobra.Command{
		{
			Use:   "serve-http",
			Short: "Run HTTP server",
			Run: func(cmd *cobra.Command, args []string) {
				if cfg.GetString("env") == "dev" {
					cleanup, err := setupProfiling(ctx, "serve-http")
					if err != nil {
						log.Fatal(err)
					}
					defer cleanup()
				}
				runHttpServerCmd(ctx)
			},
		},
		{
			Use:   "serve-queue:order",
			Short: "Run queue order server",
			Run: func(cmd *cobra.Command, args []string) {
				if cfg.GetString("env") == "dev" {
					cleanup, err := setupProfiling(ctx, "serve-queue-order")
					if err != nil {
						log.Fatal(err)
					}
					defer cleanup()
				}
				runQueueOrderCmd(ctx)
			},
		},
		{
			Use:   "serve-queue:assign-ticket",
			Short: "Run queue assign ticket server",
			Run: func(cmd *cobra.Command, args []string) {
				if cfg.GetString("env") == "dev" {
					cleanup, err := setupProfiling(ctx, "serve-queue-assign-ticket")
					if err != nil {
						log.Fatal(err)
					}
					defer cleanup()
				}
				runQueueAssignTicketCmd(ctx)
			},
		},
		{
			Use:   "serve-queue:category",
			Short: "Run queue category server",
			Run: func(cmd *cobra.Command, args []string) {
				if cfg.GetString("env") == "dev" {
					cleanup, err := setupProfiling(ctx, "serve-queue-category")
					if err != nil {
						log.Fatal(err)
					}
					defer cleanup()
				}
				runQueueCategoryCmd(ctx)
			},
		},
		{
			Use:   "serve-queue:email",
			Short: "Run queue email server",
			Run: func(cmd *cobra.Command, args []string) {
				if cfg.GetString("env") == "dev" {
					cleanup, err := setupProfiling(ctx, "serve-queue-email")
					if err != nil {
						log.Fatal(err)
					}
					defer cleanup()
				}
				runQueueEmailCmd(ctx)
			},
		},
		{
			Use:   "serve-client",
			Short: "Run client server",
			Run: func(cmd *cobra.Command, args []string) {
				if cfg.GetString("env") == "dev" {
					cleanup, err := setupProfiling(ctx, "serve-client")
					if err != nil {
						log.Fatal(err)
					}
					defer cleanup()
				}
				runClientCmd(ctx)
			},
		},
		{
			Use:   "dev",
			Short: "Run dev server, for testing purpose",
			Run: func(cmd *cobra.Command, args []string) {
				if cfg.GetString("env") == "dev" {
					cleanup, err := setupProfiling(ctx, "dev-http")
					if err != nil {
						log.Fatal(err)
					}
					defer cleanup()
				}
				runHttpServerCmd(ctx)
			},
			PreRun: func(cmd *cobra.Command, args []string) {
				go func() {
					if cfg.GetString("env") == "dev" {
						cleanup, err := setupProfiling(ctx, "dev-queue-order")
						if err != nil {
							log.Printf("Failed to setup profiling for order queue: %v", err)
							return
						}
						defer cleanup()
					}
					runQueueOrderCmd(ctx)
				}()
				go func() {
					if cfg.GetString("env") == "dev" {
						cleanup, err := setupProfiling(ctx, "dev-queue-assign-ticket")
						if err != nil {
							log.Printf("Failed to setup profiling for assign ticket queue: %v", err)
							return
						}
						defer cleanup()
					}
					runQueueAssignTicketCmd(ctx)
				}()
				go func() {
					if cfg.GetString("env") == "dev" {
						cleanup, err := setupProfiling(ctx, "dev-queue-category")
						if err != nil {
							log.Printf("Failed to setup profiling for category queue: %v", err)
							return
						}
						defer cleanup()
					}
					runQueueCategoryCmd(ctx)
				}()
				go func() {
					if cfg.GetString("env") == "dev" {
						cleanup, err := setupProfiling(ctx, "dev-client")
						if err != nil {
							log.Printf("Failed to setup profiling for client: %v", err)
							return
						}
						defer cleanup()
					}
					runClientCmd(ctx)
				}()
				//go func() {
				//	runQueueEmailCmd(ctx)
				//}()
			},
		},
	}

	rootCmd.AddCommand(cmd...)
	if err := rootCmd.Execute(); err != nil {
		log.Fatalln(err)
	}
}

// setupProfiling configures CPU and memory profiling with command-specific filenames
func setupProfiling(ctx context.Context, cmdName string) (func(), error) {
	// Create profiling directory
	err := os.MkdirAll("prof", 0755)
	if err != nil {
		return nil, fmt.Errorf("could not create prof directory: %v", err)
	}

	timestamp := time.Now().Format("20060102-150405")

	// Include command name in profiling filenames
	cpuFile := fmt.Sprintf("prof/cpu-%s-%s.prof", cmdName, timestamp)
	cpu, err := os.Create(cpuFile)
	if err != nil {
		return nil, fmt.Errorf("could not create CPU profile: %v", err)
	}

	err = pprof.StartCPUProfile(cpu)
	if err != nil {
		cpu.Close()
		return nil, fmt.Errorf("could not start CPU profile: %v", err)
	}

	memFile := fmt.Sprintf("prof/mem-%s-%s.prof", cmdName, timestamp)
	mem, err := os.Create(memFile)
	if err != nil {
		pprof.StopCPUProfile()
		cpu.Close()
		return nil, fmt.Errorf("could not create memory profile: %v", err)
	}

	// Return cleanup function to be deferred by caller
	cleanup := func() {
		pprof.StopCPUProfile()
		cpu.Close()

		if err := pprof.WriteHeapProfile(mem); err != nil {
			log.Printf("could not write memory profile: %v", err)
		}
		mem.Close()
	}

	return cleanup, nil
}
