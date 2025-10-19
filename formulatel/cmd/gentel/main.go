package main

// gentel generates formulatel data at some frequency

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/isnor/formulatel/internal"
	"github.com/isnor/formulatel/internal/genproto"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var frequency = flag.Int("frequency", 1, "frequency at which to generate telemetry; how many to generate per second")
var maxGear = flag.Int("max-gear", 7, "max gear of car")
var maxRPM = flag.Int("max-rpm", 10000, "max RPM of the car")
var acceleration = flag.Float64("acceleration", 1.0, "how fast we are accelerating")
var maxSpeed = flag.Int("max-kph", 250, "max speed of the car in k/m")

// var telemetryType = flag.String("type", "vehicle-data", "the type of telemetry to generate")

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
	tg.MaxSpeed = float32(*maxSpeed) * kphToNmMS
	tg.SpeedRate = float32(*acceleration)

	mqttConfig := mqtt.NewClientOptions().AddBroker("tcp://localhost:1883")
	mqttConfig.ClientID = "formulatel_gentel"
	topic, err := internal.NewMQTTv3Connection(mqttConfig)
	if err != nil {
		slog.ErrorContext(ctx, "failed creating mqtt connection", "error", err)
	}

	tg.GenerateLoop(ctx, func(t *genproto.VehicleData) {
		slog.InfoContext(ctx, "vroom vroom",
			"gear", t.Gear,
			"speed", t.Speed/kphToNmMS,
			"rpm", t.Rpm,
			"temp", t.EngineTemperature,
		)
		x := &genproto.GameTelemetry{
			Title:     genproto.GameTitle_GAME_TITLE_UNKNOWN,
			Timestamp: timestamppb.Now(),
			Data: &genproto.GameTelemetry_VehicleData{
				VehicleData: t,
			},
		}
		msgBytes, err := protojson.Marshal(x)
		if err != nil {
			slog.ErrorContext(ctx, "failed converting telemetry to json", "error", err)
			return
		}
		topic.Publish("formulatel/vehicle_data", 1, false, msgBytes)
	})
	<-ctx.Done()
}
