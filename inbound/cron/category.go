package cron

import (
	"concert-ticket/common"
	"concert-ticket/common/constant"
	"concert-ticket/common/vars"
	"concert-ticket/outbound/sqlgen"
	"context"
	"fmt"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"
	"log/slog"
	"strconv"
	"time"
)

type CategoryCron struct {
	Cfg     *viper.Viper
	Cache   *redis.Client
	Querier *sqlgen.Queries
}

func (in CategoryCron) Start(ctx context.Context) {
	refreshTicker := time.NewTicker(in.Cfg.GetDuration("cron.category.refresh.interval"))
	defer refreshTicker.Stop()

	// Run initial refresh
	in.refresh(ctx)

	slog.Info("category cron started")

	// Block in the main function, not in a goroutine
	for {
		select {
		case <-refreshTicker.C:
			in.refresh(ctx)
		case <-ctx.Done():
			slog.Info("category cron stopped")
			return
		}
	}
}

func (in CategoryCron) refresh(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, in.Cfg.GetDuration("cron.category.refresh.timeout"))
	defer cancel()

	traceIdAttr := common.ExtractTraceIDFromCtx(ctx)

	slog.DebugContext(ctx, "refreshing categories", traceIdAttr)

	categories := constant.CategoriesData
	quantityCacheKeys := make([]string, 0, len(categories))
	for _, category := range categories {
		quantityCacheKeys = append(quantityCacheKeys, fmt.Sprintf(constant.EachCategoryQuantityKey, category.Id))
	}

	quantities, err := in.Cache.MGet(ctx, quantityCacheKeys...).Result()
	if err != nil {
		slog.ErrorContext(ctx, "failed to get quantities from cache", traceIdAttr, slog.Any(constant.LogFieldErr, err))
		return
	}

	for i, quantity := range quantities {
		if quantity == "" {
			quantity = "0"
		}

		quantityInt, err := strconv.Atoi(quantity.(string))
		if err != nil {
			slog.ErrorContext(ctx, "failed to convert quantity to int", traceIdAttr, slog.Any(constant.LogFieldErr, err))
			return
		}

		categories[i].Quantity = int32(quantityInt)
	}

	vars.SetCategories(categories)

	slog.DebugContext(ctx, "categories refreshed successfully", traceIdAttr)
}

func (in CategoryCron) InitQuantityCache(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	categories, err := in.Querier.FindAllCategories(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to find categories", slog.Any(constant.LogFieldErr, err))
		return fmt.Errorf("find categories: %w", err)
	}

	if len(categories) == 0 {
		slog.InfoContext(ctx, "no categories found to initialize")
		return nil
	}

	pipe := in.Cache.TxPipeline()
	for _, category := range categories {
		pipe.SetNX(ctx, fmt.Sprintf(constant.EachCategoryQuantityKey, category.ID), category.Quantity, 0)
	}

	if _, err = pipe.Exec(ctx); err != nil {
		slog.ErrorContext(ctx, "failed to initialize category quantities in cache", slog.Any(constant.LogFieldErr, err))
		return fmt.Errorf("execute pipeline: %w", err)
	}

	slog.InfoContext(ctx, "category quantities initialized successfully")
	return nil
}
