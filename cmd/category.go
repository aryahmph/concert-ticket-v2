package cmd

import (
	"concert-ticket/common"
	"concert-ticket/common/constant"
	"concert-ticket/inbound/event"
	"concert-ticket/model"
	"concert-ticket/outbound/sqlgen"
	"context"
	"github.com/nats-io/nats.go/jetstream"
	"log"
	"log/slog"
	"sync"
	"time"
)

func runQueueCategoryCmd(ctx context.Context) {
	cfg := newCfg("env")

	db := newDb(cfg)
	defer db.Close()

	querier := sqlgen.New(db)

	cacheClient := newRedis(cfg)
	defer cacheClient.Close()

	natsConn := newNats(cfg)
	defer natsConn.Close()

	js := newJs(natsConn)
	createStreamWorkQueue(ctx, js)

	st, err := js.Stream(ctx, constant.QueueStreamName)
	if err != nil {
		log.Fatalln("failed to get stream", err)
	}

	incrementCategoryQuantityMessageCh := make(chan jetstream.Msg, cfg.GetInt("queue.category.increment_category_quantity_channel_size"))
	incrementCategoryQuantityTicker := time.NewTicker(cfg.GetDuration("queue.category.increment_category_quantity_interval"))

	categoryEvent := event.CategoryEvent{
		Querier: querier,
		Timeout: cfg.GetDuration("queue.category.timeout"),
	}

	cons, err := st.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Durable:       "consumer:category",
		FilterSubject: constant.CategoryWildcard,
		MaxDeliver:    cfg.GetInt("queue.category.max_deliver"),
		AckWait:       cfg.GetDuration("queue.category.ack_wait"),
	})
	if err != nil {
		log.Fatalln("failed to create consumer", err)
	}

	iter, err := cons.Messages()
	if err != nil {
		panic(err)
	}

	incrementCategoryQuantityDone := make(chan struct{})

	go func() {
		defer close(incrementCategoryQuantityMessageCh)

		for {
			select {
			case <-ctx.Done():
				return
			default:
				msg, err := iter.Next()
				if err != nil && err != jetstream.ErrMsgIteratorClosed {
					slog.ErrorContext(ctx, "Error fetching message", slog.Any(constant.LogFieldErr, err))
					continue
				}

				if msg == nil {
					continue
				}

				var eventErr error
				switch msg.Subject() {
				case constant.SubjectIncrementCategoryQuantity:
					select {
					case incrementCategoryQuantityMessageCh <- msg:
					case <-ctx.Done():
						return
					}
					continue
				case constant.SubjectBulkIncrementCategoryQuantity:
					eventErr = categoryEvent.BulkIncrementCategoryQuantityHandler(ctx, msg.Data())
				}

				if eventErr != nil {
					msg.Nak()
					continue
				}

				if err := msg.Ack(); err != nil {
					slog.ErrorContext(ctx, "Error acknowledging message",
						slog.Any(constant.LogFieldErr, err),
						slog.Any(constant.LogFieldPayload, string(msg.Data())),
						slog.String("subject", msg.Subject()),
					)
					continue
				}
			}
		}
	}()

	go func() {
		defer func() {
			close(incrementCategoryQuantityDone)
			incrementCategoryQuantityTicker.Stop()
		}()

		batchSize := cfg.GetInt("queue.category.increment_category_quantity_batch_size")
		batchMap := make(map[int16]int32)
		pendingMsgs := make([]jetstream.Msg, 0, batchSize)
		mu := sync.Mutex{}

		processBatch := func() bool {
			mu.Lock()
			if len(batchMap) == 0 {
				mu.Unlock()
				return true
			}

			localBatchMap := batchMap
			dataToSend := make([]model.IncrementCategoryQuantityEventMessage, 0, len(localBatchMap))
			for categoryID, quantity := range localBatchMap {
				if quantity != 0 {
					dataToSend = append(dataToSend, model.IncrementCategoryQuantityEventMessage{
						ID:       categoryID,
						Quantity: quantity,
					})
				}
			}

			msgsToAck := make([]jetstream.Msg, len(pendingMsgs))
			copy(msgsToAck, pendingMsgs)

			batchMap = make(map[int16]int32)
			pendingMsgs = pendingMsgs[:0]
			mu.Unlock()

			if len(dataToSend) == 0 {
				slog.InfoContext(ctx, "Skipping empty batch (zero quantities only)")
				for _, msg := range msgsToAck {
					if err := msg.Ack(); err != nil {
						slog.ErrorContext(ctx, "Error acknowledging message",
							slog.Any(constant.LogFieldErr, err),
							slog.String("subject", msg.Subject()))
					}
				}
				return true
			}

			if ctx.Err() != nil {
				return false
			}

			slog.ErrorContext(ctx, "Publishing batch", slog.Any("data", dataToSend))

			err := common.PublishMessage(ctx, js, constant.SubjectBulkIncrementCategoryQuantity, dataToSend)
			if err != nil {
				slog.ErrorContext(ctx, "failed to publish bulk increment category quantity message",
					slog.Any(constant.LogFieldErr, err),
					slog.Int("batch_size", len(dataToSend)))

				mu.Lock()
				for _, item := range dataToSend {
					batchMap[item.ID] += item.Quantity
				}
				pendingMsgs = append(pendingMsgs, msgsToAck...)
				mu.Unlock()
				return true
			}

			for _, msg := range msgsToAck {
				if err := msg.Ack(); err != nil {
					slog.ErrorContext(ctx, "Error acknowledging message",
						slog.Any(constant.LogFieldErr, err),
						slog.String("subject", msg.Subject()))
				}
			}

			return true
		}

		for {
			select {
			case <-ctx.Done():
				if len(batchMap) > 0 {
					processBatch()
				}
				return
			case <-incrementCategoryQuantityTicker.C:
				if !processBatch() {
					return
				}
			case msg := <-incrementCategoryQuantityMessageCh:
				if ctx.Err() != nil {
					return
				}

				data := categoryEvent.IncrementCategoryQuantityHandler(ctx, msg.Data())
				if data.ID == 0 {
					msg.Ack()
					continue
				}

				mu.Lock()
				batchMap[data.ID] += data.Quantity
				pendingMsgs = append(pendingMsgs, msg)
				currentBatchSize := len(pendingMsgs)
				mu.Unlock()

				if currentBatchSize >= batchSize && !processBatch() {
					return
				}
			}
		}
	}()

	slog.InfoContext(ctx, "category queue consumer started")

	<-ctx.Done()

	iter.Stop()

	<-incrementCategoryQuantityDone

	slog.InfoContext(ctx, "category queue consumer stopped")
}
