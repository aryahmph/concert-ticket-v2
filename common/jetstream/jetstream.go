package jetstream

import (
	"concert-ticket/common/constant"
	"context"
	"github.com/nats-io/nats.go/jetstream"
)

func CreateQueueStream(ctx context.Context, js jetstream.JetStream) jetstream.Stream {
	cfg := jetstream.StreamConfig{
		Name:      constant.QueueStreamName,
		Retention: jetstream.WorkQueuePolicy,
		Subjects:  []string{constant.AllWildcard},
		MaxBytes:  5 * 1024 * 1024,
	}

	st, err := js.CreateOrUpdateStream(ctx, cfg)
	if err != nil {
		panic(err)
	}

	return st
}
