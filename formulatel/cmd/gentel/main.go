package main

// gentel generates formulatel data at some frequency

import (
	"context"
	"flag"
	"os"
	"os/signal"
)

var frequency = flag.Int("frequency", 1, "frequency at which to generate telemetry; how many to generate per second")
var telemetryType = flag.String("type", "vehicle-data", "the type of telemetry to generate")


func main() {
	flag.Parse()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer cancel()

	<-ctx.Done()
}
