package cmd

import (
	"concert-ticket/common/constant"
	"concert-ticket/inbound/event"
	emailOutbound "concert-ticket/outbound/email"
	"context"
	"github.com/nats-io/nats.go/jetstream"
	"log"
	"log/slog"
)

func runQueueEmailCmd(ctx context.Context) {
	cfg := newCfg("env")

	natsConn := newNats(cfg)
	defer natsConn.Close()

	js := newJs(natsConn)
	createStreamWorkQueue(ctx, js)

	st, err := js.Stream(ctx, constant.QueueStreamName)
	if err != nil {
		log.Fatalln("failed to get stream", err)
	}

	outbound := emailOutbound.EmailOutbound{Cfg: cfg}
	outbound.Init()

	emailEvent := event.EmailEvent{
		EmailOutbound: outbound,
		Timeout:       cfg.GetDuration("queue.email.timeout"),
	}

	cons, err := st.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Durable:       "consumer:email",
		FilterSubject: constant.EmailWildcard,
		MaxDeliver:    cfg.GetInt("queue.email.max_deliver"),
		AckWait:       cfg.GetDuration("queue.email.ack_wait"),
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
				case constant.SubjectSendEmail:
					eventErr = emailEvent.SendEmailHandler(ctx, msg.Data())
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

	slog.InfoContext(ctx, "email queue consumer started")

	<-ctx.Done()

	iter.Stop()

	slog.InfoContext(ctx, "email queue consumer stopped")
}
