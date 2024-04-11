package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"time"

	"github.com/isnor/formulatel/internal/genproto/titles/f123"
	"github.com/isnor/formulatel/model"
	formulatel "github.com/isnor/formulatel/server"
)

// TODO: turns out we can't forward a UDP port in k8s without some extra stuff, so ingest needs to run on the host, not in k8s
// (unless your playstation/xbox is in the cluster)

// the largest packet is just 1460 bytes. I was trying to be cute here and only allocate
// a single packet's worth of data
const MaxPacketSize = 2048
const BufferSize = 1000 // size of the queue of packets being worked on

func main() {
	serverContext, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer cancel()
	var listener net.ListenConfig
	conn, err := listener.ListenPacket(serverContext, "udp4", "0.0.0.0:27543") // TODO: add ip/port or addr to FormulaTelIngest struct
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer conn.Close()
	// Read from UDP listener in endless loop
	println("listening on ", conn.LocalAddr().String())

	// grpc connection
	// grpcAddr := "localhost:29292"
	// backendConnection, err := grpc.DialContext(serverContext, grpcAddr,
	// 	grpc.WithTransportCredentials(insecure.NewCredentials()),
	// 	grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	// )
	// if err != nil {
	// 	log.Fatalf("could not connect to %s service, err: %+v", grpcAddr, err)
	// 	panic(err)
	// }
	// defer backendConnection.Close()

	// println("grpc connection open")

	ftClient := &FormulaTelF123Ingest{
		capture:  false,
		Shutdown: &atomic.Bool{},
	}

	var packet []byte = make([]byte, MaxPacketSize)
	var buffer chan []byte = make(chan []byte, BufferSize)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ftClient.Consume(serverContext, buffer)
	}()

	slog.InfoContext(serverContext, "starting")
	for !ftClient.Shutdown.Load() {
		select {
		// this is our "graceful shutdown" attempt
		case <-serverContext.Done():
			slog.InfoContext(serverContext, "closing server")
			ftClient.Shutdown.Store(true)
			close(buffer)
		default:
			// read a packet of data up to MaxPacketSize bytes from a UDP connection
			// TODO: there's probably a better way to handle this deadline / avoid blocking on ReadFrom, this is just the first
			if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
				println(err.Error())
			}
			numRead, _, err := conn.ReadFrom(packet)

			// per ReadFrom doc, we should check the number of bytes read before looking at the error.
			// I think it makes most sense to keep this part of the code as lean as possible to make sure we
			// can handle as many packets in as close to real-time as possible
			if numRead > 0 {
				// allocate memory for the packet and pass it to a goroutine. I think this is more performant
				// than bytes.Clone
				p := make([]byte, numRead)
				copy(p, packet)
				buffer <- p
				continue
			} else {
				slog.InfoContext(serverContext, "sleeping")
				time.Sleep(500 * time.Millisecond) // we didn't receive any bytes, wait for a second
			}

			if err != nil && !errors.Is(err, os.ErrDeadlineExceeded) {
				slog.ErrorContext(serverContext, "failed reading packets:", "error", err)
				cancel()
			}
		}
	}

	wg.Wait()
	slog.InfoContext(serverContext, "shutting down")
}

// this is fairly EA/codemasters F1-specific
func ReadBin[T any](reader io.Reader) *T {
	x := new(T)
	binary.Read(reader, binary.LittleEndian, x)
	return x
}

type FormulaTelF123Ingest struct {
	f123.CarMotionDataServiceClient
	f123.CarTelemetryDataServiceClient
	Metrics  formulatel.F123Metrics
	Shutdown *atomic.Bool
	capture  bool // TODO: remove, just for testing. when set, writes a file for every packet received
}

// Consume reads packets from a buffered channel until the channel is closed, which is a detail coupled
// with the main function: the channel should close when the program receives an interrupt. Right now,
// any messages in the buffer when the interrupt is received are lost
// TODO: is it a resource leak to not flush the channel, or will that be garbage collected? Surely all readers
// don't need to worry about flushing it
func (f *FormulaTelF123Ingest) Consume(ctx context.Context, buffer <-chan []byte) {
	slog.InfoContext(ctx, "starting reader")
	for packet := range buffer {
		f.handlePacket(ctx, packet)
	}
	slog.InfoContext(ctx, "closing reader")
}

// handlePacket reads a packet header and calls Route on the remaining bytes
func (f *FormulaTelF123Ingest) handlePacket(ctx context.Context, packet []byte) {
	if f.Shutdown.Load() {
		slog.InfoContext(ctx, "refusing to handle packet because we're already finished")
		return
	}
	var clone []byte
	if f.capture {
		clone = bytes.Clone(packet) // create a copy of packet to write to a file because we pass ownership of packet to a byte buffer; only for packet capture
	}
	buf := bytes.NewBuffer(packet)
	header := ReadBin[model.PacketHeader](buf)
	if f.capture {
		packetCapture, err := os.CreateTemp("captured_packets", fmt.Sprintf("%d_%d_%d", header.PacketId, header.SessionUID, time.Now().Nanosecond()))
		if err != nil {
			fmt.Println("failed writing capture ", err.Error())
		}
		defer packetCapture.Close()
		packetCapture.Write(clone)
	}
	f.Route(ctx, header, buf)
}

// Route uses the [PacketType] of `header` to read the bytes from `reader` into the appropriate type.
// It generally calls makes an RPC call afterwards
// TODO: consider the signature here, it was hacked together initially
func (f *FormulaTelF123Ingest) Route(ctx context.Context, header *model.PacketHeader, data *bytes.Buffer) error {

	// TODO: create a child context and add tracing
	todoContext := ctx
	switch header.PacketId {
	// case model.EventPacket:
	// 	event := ReadBin[model.EventData](data)
	// 	fmt.Printf("%+v\n%+v\n", header, event)
	case model.CarMotionPacket:
		motionArray := ReadBin[[22]model.CarMotionData](data)
		motion := motionArray[header.PlayerCarIndex]
		fmt.Printf("%+v\n%+v\n", header, motion)
		if f.CarMotionDataServiceClient != nil {
			_, err := f.CarMotionDataServiceClient.SendCarMotionData(todoContext, &f123.CarMotionData{
				WorldPositionX:     motion.WorldPositionX,
				WorldPositionY:     motion.WorldPositionY,
				WorldPositionZ:     motion.WorldPositionZ,
				WorldVelocityX:     motion.WorldVelocityX,
				WorldVelocityY:     motion.WorldVelocityY,
				WorldVelocityZ:     motion.WorldVelocityZ,
				WorldForwardDirX:   int32(motion.WorldForwardDirX),
				WorldForwardDirY:   int32(motion.WorldForwardDirY),
				WorldForwardDirZ:   int32(motion.WorldForwardDirZ),
				WorldRightDirX:     int32(motion.WorldRightDirX),
				WorldRightDirY:     int32(motion.WorldRightDirY),
				WorldRightDirZ:     int32(motion.WorldRightDirZ),
				GForceLateral:      motion.GForceLateral,
				GForceLongitudinal: motion.GForceLongitudinal,
				GForceVertical:     motion.GForceVertical,
				Yaw:                motion.Yaw,
				Pitch:              motion.Pitch,
				Roll:               motion.Roll,
			})
			if err != nil {
				fmt.Println(fmt.Errorf("oh no, an error %s", err))
			}
		}
	case model.CarTelemetryPacket:
		telemetryArray := ReadBin[[22]model.CarTelemetryData](data)
		playerTelemetry := telemetryArray[header.PlayerCarIndex]

		println(playerTelemetry.Speed)
		
		if f.CarTelemetryDataServiceClient != nil {
			f.SendCarTelemetryData(todoContext, &f123.CarTelemetryData{
				Speed:    uint32(playerTelemetry.Speed),
				Throttle: playerTelemetry.Throttle,
				Brake:    playerTelemetry.Brake,
			})
		}
	}

	return nil
}
