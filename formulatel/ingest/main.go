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
	BufferSize = 1000 // size of the queue of packets being worked on
)

func main() {
	// setup the server
	serverContext, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer cancel()
	conn, err := (&net.ListenConfig{}).ListenPacket(serverContext, "udp4", "0.0.0.0:27543") // TODO: add ip/port or addr to FormulaTelIngest struct
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer conn.Close()

	shutdown := &atomic.Bool{}
	vehicleData := make(chan *pb.GameTelemetry, 100)
	buffer := make(chan []byte, BufferSize)

	kafkaProducer := &formulatel.AsyncTelemetryProducer[*pb.GameTelemetry]{
		Writer: kafka.NewWriter(kafka.WriterConfig{
			Brokers: []string{"TODO"},
			Topic:   "motion-data", // should we have a separate topic per data per title? I guess that makes it easier to scale transformation of data easier and more modular
		}),
		Messages:  vehicleData,
		BatchSize: 12,
		Shutdown:  shutdown,
	}

	reader := &formulatel.F123PacketReader{
		Packets:            buffer,
		VehicleDataChannel: vehicleData,
		Shutdown:           shutdown,
	}

	// begin readers - we consume packets from the network, put them in a queue, and have other routines read and route them
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		reader.Consume(serverContext)
	}()
	go kafkaProducer.ProduceMessages(serverContext)

	ingestionServer := &formulatel.F123FormulaTelIngest{
		Shutdown:     shutdown,
		Server:       conn,
		Cancel:       cancel,
		PacketBuffer: buffer,
	}

	ingestionServer.Run(serverContext)
	wg.Wait()
	slog.InfoContext(serverContext, "shutting down")
}
