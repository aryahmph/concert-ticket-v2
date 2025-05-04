package cmd

import (
	"concert-ticket/common/constant"
	"concert-ticket/inbound/event"
	"concert-ticket/outbound/sqlgen"
	"context"
	"github.com/nats-io/nats.go/jetstream"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"log"
	"log/slog"
)

func runQueueAssignTicketCmd(ctx context.Context) {
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

	orderEvent := event.OrderEvent{
		Db:                   db,
		Querier:              querier,
		Publisher:            js,
		IdrCurrencyFormatter: message.NewPrinter(language.Indonesian),
		Timeout:              cfg.GetDuration("queue.order.timeout"),
	}

	cons, err := st.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Durable:       "consumer:assign-ticket",
		FilterSubject: constant.SubjectAssignOrderTicketRowCol,
		MaxDeliver:    cfg.GetInt("queue.order.max_deliver"),
		AckWait:       cfg.GetDuration("queue.order.ack_wait"),
	})
	if err != nil {
		log.Fatalln("failed to create consumer", err)
	}

	iter, err := cons.Messages(jetstream.PullMaxMessages(cfg.GetInt("queue.order.batch_size")), jetstream.PullExpiry(cfg.GetDuration("queue.order.batch_wait")))
	if err != nil {
		panic(err)
	}

	go func() {
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
				case constant.SubjectAssignOrderTicketRowCol:
					eventErr = orderEvent.AssignTicketColHandler(ctx, msg.Data())
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

	slog.InfoContext(ctx, "order queue consumer started")

	<-ctx.Done()

	iter.Stop()

	slog.InfoContext(ctx, "order queue consumer stopped")
}
