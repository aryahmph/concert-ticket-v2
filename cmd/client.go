package cmd

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

func runClientCmd(ctx context.Context) {
	cfg := newCfg("env")

	cancelTicker := time.NewTicker(cfg.GetDuration("client.cancel_interval"))
	defer cancelTicker.Stop()

	cancelUrl := cfg.GetString("client.cancel_url")

	client := &http.Client{
		Timeout: 20 * time.Second,
	}

	slog.InfoContext(ctx, "client started", slog.String("cancel_url", cancelUrl))

	go func() {
		for {
			select {
			case <-cancelTicker.C:
				go func() {
					reqCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()

					req, err := http.NewRequestWithContext(reqCtx, "POST", cancelUrl, nil)
					if err != nil {
						slog.ErrorContext(ctx, "Failed to create request",
							slog.String("url", cancelUrl),
							slog.Any("error", err))
						return
					}

					// Fire and forget - ignore response
					resp, _ := client.Do(req)
					if resp != nil {
						resp.Body.Close() // Important to prevent resource leaks
					}
				}()

			case <-ctx.Done():
				return
			}
		}
	}()

	<-ctx.Done()

	slog.InfoContext(ctx, "client stopped")
}
