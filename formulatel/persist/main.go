package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"time"

	"github.com/isnor/formulatel"
	"github.com/opensearch-project/opensearch-go/v2"
	"github.com/segmentio/kafka-go"
)

func main() {
	serverContext, _ := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})))
	shutdown := &atomic.Bool{}
	slog.InfoContext(serverContext, "starting formulatel persist")

	osClient, err := opensearch.NewClient(opensearch.Config{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Username: mustLoadEnv("OPENSEARCH_USERNAME"),
		Password: mustLoadEnv("OPENSEARCH_PASSWORD"),
	})
	if err != nil {
		slog.ErrorContext(serverContext, "failed connecting to open search")
	}

	kafkaConsumer := &formulatel.KafkaTelemetryConsumer{
		Reader: kafka.NewReader(kafka.ReaderConfig{
			Brokers: []string{"kafka:9092", "kafka:9094"},
			Topic:   formulatel.VehicleDataTopic,
			Dialer: &kafka.Dialer{
				SASLMechanism: nil,
				TLS:           nil,
				Timeout:       5 * time.Second,
			},
			MaxBytes:    2048, // game-specific
			Logger:      slog.NewLogLogger(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}), slog.LevelDebug),
			ErrorLogger: slog.NewLogLogger(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}), slog.LevelError),
		}),
	}
	// TODO: configure via cli, env, or config
	// also, we need 1 of these per topic
	kafkaToElasticSearchPersistor := &formulatel.FormulaTelPersist{
		TelemetryReader: kafkaConsumer,
		TelemetryPersistor: &formulatel.OpenSearchTelemetryPersistor{
			OpenSearch: osClient,
			Index:      formulatel.VehicleDataTopic,
		},
		Shutdown: shutdown,
	}
	slog.DebugContext(serverContext, "starting reading")

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		kafkaToElasticSearchPersistor.Run(serverContext)
	}()

	// block until interrupt cancels the context
	<-serverContext.Done()
	shutdown.Store(true)
	wg.Wait()
	slog.Info("persist shut down")
}

func mustLoadEnv(env string) string {
	value, found := os.LookupEnv(env)
	if !found {
		panic(fmt.Sprintf("could not load %s from environment", env))
	}
	return value
}
