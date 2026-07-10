package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"sync"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/isnor/formulatel/f123"
	pb "github.com/isnor/formulatel/internal/genproto"
	"github.com/isnor/formulatel/internal/mqttutil"
	"github.com/kelseyhightower/envconfig"
)

// TODO: turns out we can't forward a UDP port in k8s without some extra stuff, so ingest needs to run on the host, not in k8s
// (unless your playstation/xbox is in the cluster)
func main() {
	// setup the server
	// read environment variables, then override with CLI flags, of which we have defined none.
	// TODO: if we want to have a flag to switch on which data we're reading, we'll need to parse
	// flags before the environment variables. Shame.
	var ingestConfig f123.F123IngestConfig
	envconfig.MustProcess("formulatel", &ingestConfig)
	token := ingestConfig.Token
	ingestConfig.Token = "***"
	slog.Info("starting ingest", "config", ingestConfig)
	// TODO: we are going to need to write an actual CLI soon
	// flag.Parse()
	serverContext, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer cancel()
	// TODO: we probably shouldn't bind to 0.0.0.0
	conn, err := (&net.ListenConfig{}).ListenPacket(serverContext, "udp4", fmt.Sprintf("0.0.0.0:%d", ingestConfig.UDPPort))
	if err != nil {
		slog.Error("failed listening for UDP packets", "port", ingestConfig.UDPPort, "error", err.Error())
		os.Exit(1)
	}
	defer conn.Close()
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	ingestConfig.VehicleDataChannel = make(chan *pb.GameTelemetry, 100)
	ingestConfig.MotionDataChannel = make(chan *pb.GameTelemetry, 100)
	ingestConfig.CurrentLapDataChannel = make(chan *pb.GameTelemetry, 100)
	ingestConfig.LapTimesDataChannel = make(chan *pb.GameTelemetry, 100)
	// TODO: ingest silently failed / didn't send any telemetry when I forgot to
	// add this after implementing it in the f123 package. surely there is a better
	// design pattern for the dumbassery we're trying to do here
	ingestConfig.ExtendedWheelDataChannel = make(chan *pb.GameTelemetry, 100)

	ingest := f123.NewF123Ingest(ingestConfig, conn)

	var wg sync.WaitGroup
	wg.Go(func() {
		if err := ingest.Listen(serverContext); err != nil {
			slog.ErrorContext(serverContext, "error reading packets. stopping.", "error", err.Error())
			cancel()
		}
	})

	wg.Go(func() {
		defer func() {
			if err := recover(); err != nil {
				cancel()
				slog.Error("You've met with a terrible fate, haven't you?", "error", err)
			}
		}()
		ingest.Consume(serverContext)
	})

	// the rest of this is setting up MQTT publishers that listen to channels of GameTelemetry
	// and publish them to an MQTT topic. If we're going to support other kinds of ingestion, e.g.
	// kafka or gRPC, then we should factor this out and configure it with the CLI flags

	// Enable logging by uncommenting the below
	mqtt.ERROR = slog.NewLogLogger(slog.NewTextHandler(os.Stderr, nil), slog.LevelError)
	mqtt.DEBUG = slog.NewLogLogger(slog.NewTextHandler(os.Stdout, nil), slog.LevelDebug)

	// TODO: make mqtt options configurable
	connectionOptions := mqttutil.GenerateMQTTv3Options().AddBroker(ingestConfig.MQTTBroker)
	// TODO: this should be deterministic in some way
	connectionOptions.ClientID = ingestConfig.Username
	connectionOptions.Username = ingestConfig.Username
	connectionOptions.Password = token

	// put our telemetry type on a queue
	mqttClient, err := mqttutil.NewMQTTv3Connection(connectionOptions)
	if err != nil {
		slog.ErrorContext(serverContext, "could not connect to MQTT broker; stopping ingest", "error", err.Error())
		cancel()
	} else {

		// wire each telemetry channel with an mqtt topic
		for topic, channel := range map[string]chan *pb.GameTelemetry{
			// TODO: we need to change this to include the org and user ID for multi-tenant live viz
			// formulatel/<org>/<user>/<stream>/<title>
			"formulatel/vehicledata/f123":       ingestConfig.VehicleDataChannel,
			"formulatel/motiondata/f123":        ingestConfig.MotionDataChannel,
			"formulatel/currentlapdata/f123":    ingestConfig.CurrentLapDataChannel,
			"formulatel/laptimesdata/f123":      ingestConfig.LapTimesDataChannel,
			"formulatel/extendedwheeldata/f123": ingestConfig.ExtendedWheelDataChannel,
		} {
			wg.Go(func() {
				ctx, cancel := context.WithCancel(serverContext)
				defer cancel()
				if err := RunMQTTv3Publisher(ctx, StartPublisherConfig{
					mqttClient: mqttClient,
					data:       channel,
					topic:      topic,
				}); err != nil {
					slog.ErrorContext(ctx, "mqtt publisher failed", "error", err.Error(), "topic", topic)
				} else {
					slog.InfoContext(ctx, "started mqtt publisher", "topic", topic)
				}
			})
		}
	}
	wg.Wait()
	slog.InfoContext(serverContext, "shut down successfully")
}
