// Package main implements the persist service which subscribes to MQTT topics and writes telemetry data to TimescaleDB.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	pb "github.com/isnor/formulatel/internal/genproto"
	"github.com/isnor/formulatel/internal/mqttutil"
	"github.com/isnor/formulatel/internal/timescale"
	"github.com/kelseyhightower/envconfig"
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

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	slog.InfoContext(ctx, "starting persist service", "timescale_dsn", cfg.TimescaleDSN, "mqtt_broker", cfg.MQTTBroker)

	// Connect to TimescaleDB
	conn, err := timescale.NewConnection(ctx, cfg.TimescaleDSN)
	if err != nil {
		slog.ErrorContext(ctx, "failed to connect to timescaledb", "error", err)
		os.Exit(1)
	}
	defer func() {
		if cerr := conn.Close(ctx); cerr != nil {
			slog.ErrorContext(ctx, "failed to close connection", "error", cerr)
		}
	}()

	// Ensure schema exists
	if err := timescale.EnsureSchema(ctx, conn); err != nil {
		slog.ErrorContext(ctx, "failed to ensure schema", "error", err)
		os.Exit(1)
	}

	// Create batch router
	msgChan := make(chan *pb.GameTelemetry, msgChanBufferSize)
	router, err := timescale.NewBatchRouter(ctx, conn, msgChan, cfg.BatchSize, cfg.FlushInterval)
	if err != nil {
		slog.ErrorContext(ctx, "failed to create batch router", "error", err)
		os.Exit(1)
	}

	// Connect to MQTT
	mqttOptions := mqtt.NewClientOptions().AddBroker(cfg.MQTTBroker)
	mqttOptions.ClientID = "formulatel_persist"
	mqttOptions.SetOrderMatters(true)
	mqttOptions.ConnectRetry = true
	mqttOptions.AutoReconnect = true

	mqttClient, err := mqttutil.NewMQTTv3Connection(mqttOptions)
	if err != nil {
		slog.ErrorContext(ctx, "failed to connect to mqtt", "error", err)
		os.Exit(1)
	}

	// Subscribe to wildcard topic: formulatel/+/f123
	subTopic := cfg.MQTTPrefix + "/+/f123"
	mqttClient.Subscribe(subTopic, 0, func(client mqtt.Client, msg mqtt.Message) {
		slog.DebugContext(ctx, "received message on topic", "topic", msg.Topic())

		// Reconstruct the protobuf from JSON
		var telemetry pb.GameTelemetry
		if err := protojson.Unmarshal(msg.Payload(), &telemetry); err != nil {
			slog.ErrorContext(ctx, "failed to unmarshal mqtt message", "error", err)
			return
		}

		slog.DebugContext(ctx, "received telemetry", "session_id", telemetry.SessionId, "title", telemetry.Title)
		router.Add(&telemetry)
	})

	// Wait for disconnect or errors
	slog.InfoContext(ctx, "subscribed to mqtt topic", "topic", subTopic)
	<-ctx.Done()
	slog.InfoContext(ctx, "context cancelled, shutting down")

	// Close router to flush remaining data
	router.Close()
	slog.InfoContext(ctx, "persist service shut down successfully")
}
