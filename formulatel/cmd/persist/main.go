package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"sync"

	"github.com/isnor/formulatel"
)

func main() {
	serverContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})))
	slog.InfoContext(serverContext, "starting formulatel persist")

	// TODO: create the telemetryreader and telemetry persistory based on some kind of input?
	// remember, this is like a tool to run "formulatel persistors", it isn't a component.

	// ftelPersistor := &formulatel.FormulaTelPersist{
	// 	// TelemetryReader: nil,

	// 	// TelemetryPersistor: nil,
	// }

	var wg sync.WaitGroup
	wg.Go(func() {
		if err := listenToMQTTTopic(serverContext, formulatel.GetConnectionRequest{
			ConnectionString: "mqtt://localhost:1884",
			ClientID:         "formulatel_test_persist",
			Topic:            "formulatel/vehicle-data",
		}); err != nil {
			slog.ErrorContext(serverContext, "failed listening to topic", "error", err.Error())
			stop()
		}
	})

	// block until interrupt cancels the context
	<-serverContext.Done()
	wg.Wait()
	slog.Info("persist shut down")
}

func listenToMQTTTopic(ctx context.Context, options formulatel.GetConnectionRequest) error {
	mqc, err := formulatel.GetMqttConnection(ctx, options)
	if err != nil {
		return err
	}

	slog.Debug("reading from mqtt topic, maybe")
	<-mqc.Done()
	return nil
}
