package main

import (
	"context"
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
	shutdown := &atomic.Bool{}

	osClient, err := opensearch.NewDefaultClient()
	if err != nil {
		panic(err) // TODO: handle
	}

	kafkaConsumer := &formulatel.KafkaTelemetryConsumer{
		Reader: kafka.NewReader(kafka.ReaderConfig{
			Brokers: []string{"kafka:9092"},
			Topic:   formulatel.VehicleDataTopic,
			Dialer: &kafka.Dialer{
				SASLMechanism: nil,
				TLS:           nil,
				Timeout:       5 * time.Second,
			},
			MaxBytes:         2048, // game-specific
			QueueCapacity:    12,
			ReadBatchTimeout: time.Second,
		}),
	}

	defer kafkaConsumer.Reader.Close()
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

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		kafkaToElasticSearchPersistor.Run(serverContext)
	}()

	// block until ctrl+c cancels the context
	<-serverContext.Done()
	shutdown.Store(true)
	wg.Wait()
}
