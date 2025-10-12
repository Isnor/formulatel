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
	"github.com/isnor/formulatel"
	"github.com/isnor/formulatel/f123"
	pb "github.com/isnor/formulatel/internal/genproto"
	"google.golang.org/protobuf/encoding/protojson"
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
	// TODO: we probably shouldn't bind to 0.0.0.0 (all interfaces), but I found using 127.0.0.1 didn't work: I didn't receive
	// 	any telemetry packets
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

	reader := &f123.F123PacketReader{
		Packets:            buffer,      // read and unpack F123 packets, placing them in a data-specific channel
		VehicleDataChannel: vehicleData, // write vehicle packets as their protobuf representation here
		MotionDataChannel:  vehicleData,
	}

	// begin readers - we consume packets from the network, put them in a queue, and have other routines send them somewhere
	var wg sync.WaitGroup
	wg.Go(func() {
		defer func() {
			if err := recover(); err != nil {
				cancel()
				slog.Error("something terrible has happened", "error", err)
			}
		}()
		reader.Consume(serverContext)
	})

	mqttPublisherCtx, cancel := context.WithCancel(serverContext)
	defer cancel()

	startMQTTv3Publisher(mqttPublisherCtx, &wg, vehicleData)

	if err != nil {
		slog.ErrorContext(serverContext, err.Error())
		os.Exit(1)
	}

	f123Ingestion := &f123.F123FormulaTelIngest{
		Server:       conn,
		Cancel:       cancel,
		PacketBuffer: buffer, // send all packets here
	}

	f123Ingestion.Run(serverContext)
	wg.Wait()
	slog.InfoContext(serverContext, "shut down succesfully")
}

// startMQTTv3Publisher reads data from `dataChan` and publishes it to an MQTT topic
func startMQTTv3Publisher(ctx context.Context, wg *sync.WaitGroup, dataChan <-chan *pb.GameTelemetry) error {
	// Enable logging by uncommenting the below
	mqttv3.ERROR = slog.NewLogLogger(slog.NewTextHandler(os.Stderr, nil), slog.LevelError)
	mqttv3.DEBUG = slog.NewLogLogger(slog.NewTextHandler(os.Stdout, nil), slog.LevelDebug)
	// mqtt.CRITICAL = slog.NewLogLogger()
	// mqtt.WARN = slog.NewLogLogger()

	// TODO: make it configurable
	connectionOptions := mqttv3.NewClientOptions().AddBroker("tcp://localhost:1883")
	connectionOptions.ClientID = "formulatel_ingest"
	mqttClient, err := formulatel.NewMQTTv3Connection(connectionOptions)
	if err != nil {
		return err
	}

	wg.Go(func() {
		for {
			select {
			case <-ctx.Done():
				slog.InfoContext(ctx, "finished publishing to mqtt")
				return
			case data := <-dataChan:
				protoBytes, err := protojson.Marshal(data)
				if err != nil {
					// TODO: handle better
					slog.ErrorContext(ctx, "mqtt ingest failed serializing a message")
					continue
				}
				slog.DebugContext(ctx, "mqtt ingest read a packet")
				// TODO: make it configurable
				if token := mqttClient.Publish("formulatel/vehicledata", 1, false, protoBytes); !token.Wait() || token.Error() != nil {
					slog.ErrorContext(ctx, "v3 client failed publishing", "error", token.Error())
				}
				slog.DebugContext(ctx, "published vehicle data to mqtt topic")
			}
		}
	})

	return nil
}
