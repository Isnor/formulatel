package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"

	"github.com/isnor/formulatel"
)

func main() {
	serverContext, _ := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})))
	shutdown := &atomic.Bool{}
	slog.InfoContext(serverContext, "starting formulatel persist")

	// TODO: create the telemetryreader and telemetry persistory based on some kind of input?

	ftelPersistor := &formulatel.FormulaTelPersist{
		// TelemetryReader: nil,

		// TelemetryPersistor: nil,
		Shutdown: shutdown,
	}
	slog.DebugContext(serverContext, "starting reading")

	var wg sync.WaitGroup
	wg.Go(func() {
		ftelPersistor.Run(serverContext)
	})

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
