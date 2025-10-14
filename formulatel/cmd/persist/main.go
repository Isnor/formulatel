package main

import (
	"log/slog"
)

func main() {
	// serverContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	// defer stop()
	// slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
	// 	Level: slog.LevelDebug,
	// })))
	// slog.InfoContext(serverContext, "starting formulatel persist")

	// var wg sync.WaitGroup

	// wg.Wait()
	slog.Info("persist shut down")
}
