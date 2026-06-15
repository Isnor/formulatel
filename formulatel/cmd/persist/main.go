// Package main implements the persist service which subscribes to MQTT topics and writes telemetry data to TimescaleDB.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	pb "github.com/isnor/formulatel/internal/genproto"
	"github.com/isnor/formulatel/internal/mqttutil"
	"github.com/isnor/formulatel/internal/timescale"
	"github.com/kelseyhightower/envconfig"
	"go.opentelemetry.io/otel"

	"go.opentelemetry.io/contrib/exporters/autoexport"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
	"google.golang.org/protobuf/encoding/protojson"
)

const (
	msgChanBufferSize = 100
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	var cfg timescale.Config
	err := envconfig.Process("formulatel", &cfg)
	if err != nil {
		slog.ErrorContext(ctx, "failed to load config", "error", err)
		os.Exit(1)
	}

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	slog.InfoContext(ctx, "starting persist service", "timescale_dsn", cfg.TimescaleDSN, "mqtt_broker", cfg.MQTTBroker)

	// TODO: we shouldn't need this once we get OBI + auto working, but I've been having trouble and just want to see my traces
	exporter, err := autoexport.NewSpanExporter(ctx)
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(.1))),
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource.NewWithAttributes(semconv.SchemaURL, semconv.ServiceName("formulatel-persist"))),
	)
	otel.SetTracerProvider(tracerProvider)

	// Connect to TimescaleDB
	connPool, err := timescale.NewConnectionPool(ctx, cfg.TimescaleDSN)
	if err != nil {
		slog.ErrorContext(ctx, "failed to connect to timescaledb", "error", err)
		os.Exit(1)
	}
	defer func() {
		connPool.Close()
	}()

	// Create batch router
	msgChan := make(chan *pb.GameTelemetry, msgChanBufferSize)
	router, err := timescale.NewBatchRouter(ctx, connPool, msgChan, cfg.BatchSize, cfg.FlushInterval)
	if err != nil {
		slog.ErrorContext(ctx, "failed to create batch router", "error", err)
		os.Exit(1)
	}

	mqtt.ERROR = slog.NewLogLogger(slog.NewTextHandler(os.Stderr, nil), slog.LevelError)
	mqtt.DEBUG = slog.NewLogLogger(slog.NewTextHandler(os.Stdout, nil), slog.LevelDebug)
	// Connect to MQTT
	mqttOptions := mqtt.NewClientOptions().AddBroker(cfg.MQTTBroker)
	// TODO: should this be configurable?
	mqttOptions.ClientID = "formulatel_persist"
	mqttOptions.SetOrderMatters(false)
	mqttOptions.ConnectRetry = true
	mqttOptions.AutoReconnect = true

	mqttClient, err := mqttutil.NewMQTTv3Connection(mqttOptions)
	if err != nil {
		slog.ErrorContext(ctx, "failed to connect to mqtt", "error", err)
		os.Exit(1)
	}

	// Subscribe to wildcard topic: formulatel/+/f123
	subTopic := cfg.MQTTPrefix + "/+/f123"
	tracer := otel.Tracer("formulatel/persist/mqtt")
	mqttClient.Subscribe(subTopic, 0, func(client mqtt.Client, msg mqtt.Message) {
		msgCtx, span := tracer.Start(ctx, "mqtt.receive")
		defer span.End()
		slog.DebugContext(msgCtx, "received message on topic", "topic", msg.Topic())

		// Reconstruct the protobuf from JSON
		// TODO: see notes on transport in the readme
		var telemetry pb.GameTelemetry
		if err := protojson.Unmarshal(msg.Payload(), &telemetry); err != nil {
			slog.ErrorContext(msgCtx, "failed to unmarshal mqtt message", "error", err)
			return
		}

		slog.DebugContext(msgCtx, "received telemetry", "session_id", telemetry.SessionId, "title", telemetry.Title)
		// Pass msgCtx and topic for trace propagation
		router.Add(msgCtx, &telemetry)
	})

	// Wait for disconnect or errors
	slog.InfoContext(ctx, "subscribed to mqtt topic", "topic", subTopic)
	<-ctx.Done()
	slog.InfoContext(ctx, "shutting down...")
	time.Sleep(time.Millisecond * 500)
	slog.InfoContext(ctx, "persist service shut down successfully")
}
