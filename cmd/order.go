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
	"os"
	"runtime/pprof"
	"time"
)

func runQueueOrderCmd(ctx context.Context) {
	cfg := newCfg("env")

	if cfg.GetString("env") == "dev" {
		cpu, err := os.Create("order-cpu.prof")
		if err != nil {
			log.Fatalf("could not create CPU profile: %v", err)
		}
		defer cpu.Close()

		err = pprof.StartCPUProfile(cpu)
		if err != nil {
			log.Fatalf("could not start CPU profile: %v", err)
		}
		defer pprof.StopCPUProfile()

		mem, err := os.Create("order-mem.prof")
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
		Durable:       "consumer:order",
		FilterSubject: constant.OrderWildcard,
		MaxDeliver:    cfg.GetInt("queue.order.max_deliver"),
		AckWait:       cfg.GetDuration("queue.order.ack_wait"),
	})
	if err != nil {
		log.Fatalln("failed to create consumer", err)
	}

	iter, err := cons.Messages()
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
				case constant.SubjectCreateOrder:
					eventErr = orderEvent.CreateHandler(ctx, msg.Data())
				case constant.SubjectCallbackPayment:
					eventErr = orderEvent.CompleteHandler(ctx, msg.Data())
				case constant.SubjectAssignOrderTicketRowCol:
					eventErr = orderEvent.AssignTicketColHandler(ctx, msg.Data())
				}

				if eventErr != nil {
					msg.NakWithDelay(1 * time.Second)
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
