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

	mqttv3 "github.com/eclipse/paho.mqtt.golang"
	"github.com/isnor/formulatel/f123"
	"github.com/isnor/formulatel/internal"
	pb "github.com/isnor/formulatel/internal/genproto"
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

	// TODO: make this configurable
	vehicleData := make(chan *pb.GameTelemetry, 100)
	buffer := make(chan []byte, BufferSize)
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})))
	var wg sync.WaitGroup

	// start reading packets
	f123Ingestion := &f123.F123PacketReader{
		Server:       conn,
		PacketBuffer: buffer, // send all packets here
	}

	wg.Go(func() {
		if err := f123Ingestion.Run(serverContext); err != nil {
			slog.ErrorContext(serverContext, "error reading packets. stopping.", "error", err.Error())
			cancel()
		}
	})

	// transform packets into our telemetry type
	transformer := &f123.F123PacketTransformer{
		Packets:            buffer,      // read and unpack F123 packets, placing them in a data-specific channel
		VehicleDataChannel: vehicleData, // write vehicle packets as their protobuf representation here
		// MotionDataChannel:  vehicleData,
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

	mqttPublisherCtx, cancel := context.WithCancel(serverContext)
	defer cancel()

	// Enable logging by uncommenting the below
	mqttv3.ERROR = slog.NewLogLogger(slog.NewTextHandler(os.Stderr, nil), slog.LevelError)
	mqttv3.DEBUG = slog.NewLogLogger(slog.NewTextHandler(os.Stdout, nil), slog.LevelDebug)
	// mqtt.CRITICAL = slog.NewLogLogger()
	// mqtt.WARN = slog.NewLogLogger()

	// TODO: make it configurable
	connectionOptions := mqttv3.NewClientOptions().AddBroker("tcp://localhost:1883")
	connectionOptions.ClientID = "formulatel_ingest"

	// put our telemetry type on a queue
	mqttClient, err := internal.NewMQTTv3Connection(connectionOptions)
	if err != nil {
		slog.ErrorContext(serverContext, err.Error())
		cancel()
		os.Exit(1)
	}

	wg.Go(func() {
		if err := internal.StartMQTTv3Publisher(mqttPublisherCtx, mqttClient, vehicleData); err != nil {
			slog.ErrorContext(mqttPublisherCtx, "mqtt publisher failed", "error", err.Error())
		}
	})

	wg.Wait()
	slog.InfoContext(serverContext, "shut down successfully")
}
