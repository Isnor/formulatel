package main

import (
	"context"
	"flag"
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
)

// TODO: turns out we can't forward a UDP port in k8s without some extra stuff, so ingest needs to run on the host, not in k8s
// (unless your playstation/xbox is in the cluster)

const (
	BufferSize    = 1000  // size of the queue of packets being worked on
	TelemetryPort = 27543 // chosen at "random"
)

func main() {
	// setup the server
	flag.Parse()
	serverContext, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer cancel()
	// TODO: we probably shouldn't bind to 0.0.0.0
	conn, err := (&net.ListenConfig{}).ListenPacket(serverContext, "udp4", fmt.Sprintf("0.0.0.0:%d", TelemetryPort))
	if err != nil {
		slog.Error("failed listening for UDP packets", "port", TelemetryPort, "error", err.Error())
		os.Exit(1)
	}
	defer conn.Close()
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	// TODO: make this configurable; all of this should be in f123 and ingest should have a flag in the CLI
	//	that determines which packets it is ingesting
	vehicleData := make(chan *pb.GameTelemetry, 100)
	motionData := make(chan *pb.GameTelemetry, 100)
	currentLapData := make(chan *pb.GameTelemetry, 100)
	lapTimesData := make(chan *pb.GameTelemetry, 100)
	buffer := make(chan []byte, BufferSize)

	// start reading packets
	f123Ingestion := &f123.F123PacketReader{
		Server:       conn,
		PacketBuffer: buffer, // send all packets here
	}

	var wg sync.WaitGroup
	wg.Go(func() {
		if err := f123Ingestion.Run(serverContext); err != nil {
			slog.ErrorContext(serverContext, "error reading packets. stopping.", "error", err.Error())
			cancel()
		}
	})

	// transform packets into our telemetry type
	transformer := &f123.F123PacketTransformer{
		Packets:               buffer,         // read and unpack F123 packets, placing them in a data-specific channel
		VehicleDataChannel:    vehicleData,    // write vehicle packets as their protobuf representation here
		MotionDataChannel:     motionData,     // write motion packets as their protobuf representation here
		CurrentLapDataChannel: currentLapData, // write current lap times packets as their protobuf representation here
		LapTimesDataChannel:   lapTimesData,   // write historic lap data packets as their protobuf representation here
		LatestLaps:            f123.NewLatestLapData(),
	}

	wg.Go(func() {
		defer func() {
			if err := recover(); err != nil {
				cancel()
				slog.Error("something terrible has happened", "error", err)
			}
		}()
		transformer.Consume(serverContext)
	})

	// the rest of this is setting up MQTT publishers that listen to channels of GameTelemetry
	// and publish them to an MQTT topic. If we're going to support other kinds of ingestion, e.g.
	// kafka or gRPC, then we should factor this out and configure it with the CLI flags
	mqttPublisherCtx, cancel := context.WithCancel(serverContext)
	defer cancel()

	// Enable logging by uncommenting the below
	mqtt.ERROR = slog.NewLogLogger(slog.NewTextHandler(os.Stderr, nil), slog.LevelError)
	mqtt.DEBUG = slog.NewLogLogger(slog.NewTextHandler(os.Stdout, nil), slog.LevelDebug)
	// mqtt.CRITICAL = slog.NewLogLogger()
	// mqtt.WARN = slog.NewLogLogger()

	// TODO: make broker configurable
	connectionOptions := mqtt.NewClientOptions().AddBroker("tcp://localhost:1883")
	// TODO: this should be deterministic in some way
	connectionOptions.ClientID = "formulatel_ingest"

	// put our telemetry type on a queue
	mqttClient, err := mqttutil.NewMQTTv3Connection(connectionOptions)
	if err != nil {
		slog.ErrorContext(serverContext, err.Error())
		cancel()
		os.Exit(1)
	}

	wg.Go(func() {
		ctx, cancel := context.WithCancel(mqttPublisherCtx)
		defer cancel()
		// start a routine to read f123-specific [VehicleData] into a hard-coded topic `formulatel/vehicledata/f123`
		if err := StartMQTTv3Publisher(ctx, StartPublisherConfig{
			mqttClient: mqttClient,
			data:       vehicleData,
			topic:      "formulatel/vehicledata/f123",
		}); err != nil {
			slog.ErrorContext(ctx, "mqtt publisher failed", "error", err.Error())
		}
	})

	wg.Go(func() {
		ctx, cancel := context.WithCancel(mqttPublisherCtx)
		defer cancel()
		// start a routine to read f123-specific [MotionData] into a hard-coded topic `formulatel/motiondata/f123`
		if err := StartMQTTv3Publisher(ctx, StartPublisherConfig{
			mqttClient: mqttClient,
			data:       motionData,
			topic:      "formulatel/motiondata/f123",
		}); err != nil {
			slog.ErrorContext(ctx, "mqtt publisher failed", "error", err.Error())
		}
	})

	wg.Go(func() {
		ctx, cancel := context.WithCancel(mqttPublisherCtx)
		defer cancel()
		// start a routine to read f123-specific [LapTimesData] into a hard-coded topic `formulatel/currentlapdata/f123`
		if err := StartMQTTv3Publisher(ctx, StartPublisherConfig{
			mqttClient: mqttClient,
			data:       currentLapData,
			topic:      "formulatel/currentlapdata/f123",
		}); err != nil {
			slog.ErrorContext(ctx, "mqtt publisher failed", "error", err.Error())
		}
	})

	wg.Go(func() {
		ctx, cancel := context.WithCancel(mqttPublisherCtx)
		defer cancel()
		// start a routine to read f123-specific [SessionHistory] into a hard-coded topic `formulatel/laptimesdata/f123`
		if err := StartMQTTv3Publisher(ctx, StartPublisherConfig{
			mqttClient: mqttClient,
			data:       lapTimesData,
			topic:      "formulatel/laptimesdata/f123",
		}); err != nil {
			slog.ErrorContext(ctx, "mqtt publisher failed", "error", err.Error())
		}
	})

	wg.Wait()
	slog.InfoContext(serverContext, "shut down successfully")
}
