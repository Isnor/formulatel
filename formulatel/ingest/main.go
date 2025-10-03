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
	"sync/atomic"

	"github.com/isnor/formulatel"
	pb "github.com/isnor/formulatel/internal/genproto"
)

// TODO: turns out we can't forward a UDP port in k8s without some extra stuff, so ingest needs to run on the host, not in k8s
// (unless your playstation/xbox is in the cluster)

const (
	BufferSize    = 1000  // size of the queue of packets being worked on
	TelemetryPort = 27543 // chosen at "random"
)

var (
	port = flag.Int("kafka-port", 1234, "kafka broker port")
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

	shutdown := &atomic.Bool{}
	// TODO: make this configurable
	vehicleData := make(chan *pb.GameTelemetry, 100)
	buffer := make(chan []byte, BufferSize)
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})))

	reader := &formulatel.F123PacketReader{
		Packets:            buffer,      // read and unpack F123 packets, placing them in a data-specific channel
		VehicleDataChannel: vehicleData, // write motion packets as their protobuf representation here
		Shutdown:           shutdown,
	}

	// begin readers - we consume packets from the network, put them in a queue, and have other routines send them somewhere
	var wg sync.WaitGroup
	wg.Go(func() {
		reader.Consume(serverContext)
	})

	// what was the plan for extension here exactly? Just tell developers to write their own version of this file?

	mqttConnection, err := formulatel.GetMqttConnection(formulatel.GetConnectionRequest{
		Context: serverContext,
		// TODO: make command line flags for this because we had to forward the port manually/via k9s
		ConnectionString: "mqtt://localhost:1884",
		ClientID:         "formulatel_test",
	})

	if err != nil {
		slog.ErrorContext(serverContext, err.Error())
		os.Exit(1)
	}

	mqttFormulatelIngestor := formulatel.MQTTFormulatelIngest{
		MQTT:     mqttConnection,
		Messages: vehicleData,
	}

	// TODO: if we wanted to handle sending telemetry via an API or some other queue other than Kafka, this is what
	//	we would change. It's not exactly a drop-in replacement the way I'd like it to be eventually, but that's life.
	// 	The contract is really just "read packets from this channel (buffer) and do something with them"
	wg.Go(func() {
		mqttFormulatelIngestor.Run(serverContext, "formulatel/vehicle-data")
	})

	f123Ingestion := &formulatel.F123FormulaTelIngest{
		Shutdown:     shutdown,
		Server:       conn,
		Cancel:       cancel,
		PacketBuffer: buffer, // send all packets here
	}

	f123Ingestion.Run(serverContext)
	wg.Wait()
	// vehicleDataKafkaProducer.Writer.Close()
	slog.InfoContext(serverContext, "shut down succesfully")
}
