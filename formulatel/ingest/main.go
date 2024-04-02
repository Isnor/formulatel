package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"time"

	"github.com/isnor/formulatel/internal/genproto/titles/f123"
	"github.com/isnor/formulatel/model"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// TODO: turns out we can't forward a UDP port in k8s without some extra stuff, so ingest needs to run on the host, not in k8s
// (unless your playstation/xbox is in the cluster)

// the largest packet is just 1460 bytes. I was trying to be cute here and only allocate
// a single packet's worth of data, but the main function indiscriminately allocates as many bytes
// as it needs to read each packet as quickly as possible. This probably doesn't scale very well
// and we should use some kind of pool of routines / memory so that GC overhead doesn't become an 
// issue.
const MaxPacketSize = 2048

func main() {
	var listener net.ListenConfig
	conn, err := listener.ListenPacket(context.TODO(), "udp4", "0.0.0.0:27543") // TODO: add ip/port or addr to FormulaTelIngest struct
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer conn.Close()
	// Read from UDP listener in endless loop
	println("listening on ", conn.LocalAddr().String())

	// grpc connection
	grpcAddr := "localhost:29292"
	backendConnection, err := grpc.DialContext(context.Background(), grpcAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	)
	if err != nil {
		log.Fatalf("could not connect to %s service, err: %+v", grpcAddr, err)
		panic(err)
	}
	defer backendConnection.Close()

	println("grpc connection open")

	ftClient := &FormulaTelIngest{
		capture:                       false,
		CarMotionDataServiceClient:    f123.NewCarMotionDataServiceClient(backendConnection),
		CarTelemetryDataServiceClient: f123.NewCarTelemetryDataServiceClient(backendConnection),
	}

	var packet []byte = make([]byte, MaxPacketSize)
	for {
		// read a packet of data up to BufferSize bytes from a UDP connection
		numRead, _, err := conn.ReadFrom(packet)

		// per ReadFrom doc, we should check the number of bytes read before looking at the error.
		// I think it makes most sense to keep this part of the code as lean as possible to make sure we
		// can handle as many packets in as close to real-time as possible
		if numRead > 0 {
			// allocate memory for the packet and pass it to a goroutine. I think this is more performant
			// than bytes.Clone
			p := make([]byte, numRead)
			copy(p, packet)
			go ftClient.handlePacket(p)
		}

		if err != nil {
			panic(err)
		}
	}

}

// this is fairly EA/codemasters F1-specific
func ReadBin[T any](reader io.Reader) *T {
	x := new(T)
	binary.Read(reader, binary.LittleEndian, x)
	return x
}

type FormulaTelIngest struct {
	f123.CarMotionDataServiceClient
	f123.CarTelemetryDataServiceClient
	capture bool // TODO: remove, just for testing. when set, writes a file for every packet received
}

// handlePacket reads a packet header and calls Route on the remaining bytes
func (f *FormulaTelIngest) handlePacket(packet []byte) {
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
	f.Route(header, buf)
}

// Route uses the [PacketType] of `header` to read the bytes from `reader` into the appropriate type.
// It generally calls makes an RPC call afterwards
func (f *FormulaTelIngest) Route(header *model.PacketHeader, data *bytes.Buffer) error {

	switch header.PacketId {
	// case model.EventPacket:
	// 	event := ReadBin[model.EventData](data)
	// 	fmt.Printf("%+v\n%+v\n", header, event)
	case model.CarMotionPacket:
		motionArray := ReadBin[[22]model.CarMotionData](data)
		motion := motionArray[0]
		fmt.Printf("%+v\n%+v\n", header, motion)
		if f.CarMotionDataServiceClient != nil {
			// TODO: we could put a trace on the context here; could be fun, just to learn a bit more about it and visualizing the traces
			_, err := f.CarMotionDataServiceClient.SendCarMotionData(context.TODO(), &f123.CarMotionData{
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
		playerTelemetry := telemetryArray[0]
		println(playerTelemetry.Speed)
		if f.CarTelemetryDataServiceClient != nil {
			f.SendCarTelemetryData(context.TODO(), &f123.CarTelemetryData{
				Speed:    uint32(playerTelemetry.Speed),
				Throttle: playerTelemetry.Throttle,
				Brake:    playerTelemetry.Brake,
			})
		}
	}

	return nil
}
