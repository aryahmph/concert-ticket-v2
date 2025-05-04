package cmd

import (
	"concert-ticket/common/constant"
	"concert-ticket/common/otel"
	"context"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"
	"log"
	"os"
)

func newCfg(name string) *viper.Viper {
	config := viper.New()

	config.SetConfigName(name)
	config.SetConfigType("yaml")
	config.AddConfigPath(".")

	err := config.ReadInConfig()
	if err != nil {
		log.Fatalln(err)
	}

	err = os.Setenv("TZ", config.GetString("server.timezone"))
	if err != nil {
		log.Fatalln(err)
	}

	return config
}

func newDb(cfg *viper.Viper) *pgxpool.Pool {
	username := cfg.GetString("db.user")
	password := cfg.GetString("db.password")
	host := cfg.GetString("db.host")
	port := cfg.GetInt("db.port")
	database := cfg.GetString("db.name")
	maxConn := cfg.GetInt("db.pool.max")
	minConn := cfg.GetInt("db.pool.min")
	timezone := cfg.GetString("server.timezone")

	connString := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?timezone=%s",
		username, password, host, port, database, timezone)

	config, err := pgxpool.ParseConfig(connString)
	if err != nil {
		log.Fatalln(err)
	}

	config.MaxConns = int32(maxConn)
	config.MinConns = int32(minConn)
	config.ConnConfig.Tracer = &otel.PgxCustomTracer{}

	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		log.Fatalln(err)
	}

	err = pool.Ping(context.Background())
	if err != nil {
		log.Fatalln(err)
	}

	return pool
}

func newRedis(cfg *viper.Viper) *redis.Client {
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.GetString("redis.addr"),
		Password: cfg.GetString("redis.password"),
		DB:       0,
	})

	err := rdb.Ping(context.Background()).Err()
	if err != nil {
		log.Fatalln(err)
	}

	return rdb
}

func newNats(viper *viper.Viper) *nats.Conn {
	conn, err := nats.Connect(viper.GetString("nats.addr"))
	if err != nil {
		log.Fatalln(err)
	}

	return conn
}

func newJs(conn *nats.Conn) jetstream.JetStream {
	js, err := jetstream.New(conn)
	if err != nil {
		log.Fatalln(err)
	}

	return js
}

func createStreamWorkQueue(ctx context.Context, js jetstream.JetStream) jetstream.Stream {
	cfg := jetstream.StreamConfig{
		Name:      constant.QueueStreamName,
		Retention: jetstream.WorkQueuePolicy,
		Subjects:  []string{constant.AllWildcard},
		MaxBytes:  -1,
	}

	st, err := js.CreateOrUpdateStream(ctx, cfg)
	if err != nil {
		panic(err)
	}

	return st
}
