package cmd

import (
	"context"
	"github.com/spf13/cobra"
	"log"
	"log/slog"
	"os"
	"os/signal"
)

func Start() {
	cfg := newCfg("env")
	slog.SetLogLoggerLevel(slog.Level(cfg.GetInt("log.level")))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	rootCmd := &cobra.Command{}
	cmd := []*cobra.Command{
		{
			Use:   "serve-http",
			Short: "Run HTTP server",
			Run: func(cmd *cobra.Command, args []string) {
				runHttpServerCmd(ctx)
			},
		},
		{
			Use:   "serve-queue:order",
			Short: "Run queue order server",
			Run: func(cmd *cobra.Command, args []string) {
				runQueueOrderCmd(ctx)
			},
		},
		{
			Use:   "serve-queue:category",
			Short: "Run queue category server",
			Run: func(cmd *cobra.Command, args []string) {
				runQueueCategoryCmd(ctx)
			},
		},
		{
			Use:   "serve-queue:email",
			Short: "Run queue email server",
			Run: func(cmd *cobra.Command, args []string) {
				runQueueEmailCmd(ctx)
			},
		},
		{
			Use:   "dev",
			Short: "Run dev server, for testing purpose",
			Run: func(cmd *cobra.Command, args []string) {
				runHttpServerCmd(ctx)
			},
			PreRun: func(cmd *cobra.Command, args []string) {
				go func() {
					runQueueOrderCmd(ctx)
				}()
				go func() {
					runQueueCategoryCmd(ctx)
				}()
				go func() {
					runQueueEmailCmd(ctx)
				}()
			},
		},
	}

	rootCmd.AddCommand(cmd...)
	if err := rootCmd.Execute(); err != nil {
		log.Fatalln(err)
	}
}
