package main

// gentel generates formulatel data at some frequency

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"

	"github.com/isnor/formulatel/internal/genproto"
)

var frequency = flag.Int("frequency", 1, "frequency at which to generate telemetry; how many to generate per second")
var maxGear = flag.Int("max-gear", 7, "max gear of car")
var maxRPM = flag.Int("max-rpm", 10000, "max RPM of the car")
var acceleration = flag.Float64("acceleration", 1.0, "how fast we are accelerating")
var maxSpeed = flag.Int("max-kph", 250, "max speed of the car in k/m")

// var telemetryType = flag.String("type", "vehicle-data", "the type of telemetry to generate")

// -frequency=10 -max-gear=10 -max-rpm=15000 -max-kph=400 -acceleration=.1 ended up being OK
func main() {
	flag.Parse()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer cancel()
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})))
	tg := NewTelemetryGenerator(*frequency)
	tg.MaxGear = uint32(*maxGear)
	tg.MaxRPM = uint32(*maxRPM)
	tg.MaxSpeed = float32(*maxSpeed)
	tg.SpeedRate = float32(*acceleration)
	tg.GenerateLoop(ctx, func(t *genproto.VehicleData) {
		// TODO: put on mqtt topic
		slog.InfoContext(ctx, "vroom vroom", "gear", t.Gear, "speed", t.Speed, "rpm", t.Rpm, "temp", t.EngineTemperature)
	})
	<-ctx.Done()
}
