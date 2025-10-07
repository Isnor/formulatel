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

	ftelPersistor := &formulatel.FormulaTelPersist{
		// TelemetryReader: nil,

		// TelemetryPersistor: nil,
	}
	slog.DebugContext(serverContext, "starting reading")

	var wg sync.WaitGroup
	wg.Go(func() {
		if err := ftelPersistor.Run(serverContext); err != nil {
			stop()
		}
	})

	// block until interrupt cancels the context
	<-serverContext.Done()
	wg.Wait()
	slog.Info("persist shut down")
}
