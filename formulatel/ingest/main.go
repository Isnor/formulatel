package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"

	"github.com/isnor/formulatel"
	pb "github.com/isnor/formulatel/internal/genproto"
	"github.com/segmentio/kafka-go"
)

// TODO: turns out we can't forward a UDP port in k8s without some extra stuff, so ingest needs to run on the host, not in k8s
// (unless your playstation/xbox is in the cluster)

const (
	BufferSize    = 1000  // size of the queue of packets being worked on
	TelemetryPort = 27543 // chosen at "random"
)

func main() {
	// setup the server
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
	vehicleData := make(chan *pb.GameTelemetry, 100)
	buffer := make(chan []byte, BufferSize)
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})))
	vehicleDataKafkaProducer := &formulatel.KafkaTelemetryProducer{
		Writer: &kafka.Writer{
			// TODO: I don't want to think about auth right now, so future Me, it's your problem now
			Addr: kafka.TCP("localhost:9092", "localhost:9093", "localhost:9094"),
			Transport: &kafka.Transport{
				Dial: kafka.DefaultDialer.DialFunc,
				SASL: nil,
				TLS:  nil,
			},
			Topic:        formulatel.VehicleDataTopic,
			BatchSize:    12,
			BatchBytes:   2048 * 12,
			RequiredAcks: kafka.RequireNone,
			Async:        true,
			Balancer:     kafka.Murmur2Balancer{},
			Logger:       slog.NewLogLogger(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}), slog.LevelInfo),
			ErrorLogger:  slog.NewLogLogger(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}), slog.LevelError),
		},
		Messages: vehicleData,
		Shutdown: shutdown,
	}

	reader := &formulatel.F123PacketReader{
		Packets:            buffer,      // read and unpack F123 packets, placing them in a data-specific channel
		VehicleDataChannel: vehicleData, // write motion packets as their protobuf representation here
		Shutdown:           shutdown,
	}

	// begin readers - we consume packets from the network, put them in a queue, and have other routines send them somewhere
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		reader.Consume(serverContext)
	}()

	// TODO: if we wanted to handle sending telemetry via an API or some other queue other than Kafka, this is what
	//	we would change. It's not exactly a drop-in replacement the way I'd like it to be eventually, but that's life.
	// 	The contract is really just "read packets from this channel (buffer) and do something with them"
	wg.Add(1)
	go func() {
		defer wg.Done()
		vehicleDataKafkaProducer.ProduceMessages(serverContext)
	}()

	f123Ingestion := &formulatel.F123FormulaTelIngest{
		Shutdown:     shutdown,
		Server:       conn,
		Cancel:       cancel,
		PacketBuffer: buffer, // send all packets here
	}

	f123Ingestion.Run(serverContext)
	wg.Wait()
	vehicleDataKafkaProducer.Writer.Close()
	slog.InfoContext(serverContext, "shutting down")
}
