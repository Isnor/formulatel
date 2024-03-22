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

	pb "github.com/isnor/formulatel/internal/genproto"
	"github.com/isnor/formulatel/model"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// the largest packet is just 1460 bytes
const BufferSize = 2048

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

	// TODO: setup otel and shove it in this context (or whatever)
	// TODO: load this from env
	// grpc connection
	grpcAddr := "localhost:29292"
	c, err := grpc.DialContext(context.TODO(), grpcAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	)
	if err != nil {
		log.Fatalf("could not connect to %s service, err: %+v", grpcAddr, err)
		panic(err)
	}
	defer c.Close()

	println("grpc connection open")

	ftClient := &FormulaTelIngest{
		capture:                    false,
		CarMotionDataServiceClient: pb.NewCarMotionDataServiceClient(c),
	}

	var packet []byte = make([]byte, BufferSize)
	for {
		// read a packet of data up to BufferSize bytes from a UDP connection
		numRead, _, err := conn.ReadFrom(packet)

		// per ReadFrom doc, we should check the number of bytes read before looking at the error.
		// I think it makes most sense to keep this part of the code as lean as possible to make sure we
		// can handle as many packets in as close to real-time as possible
		if numRead > 0 {
			// copy the packet into a new array
			myPacket := make([]byte, numRead)
			copy(myPacket, packet)
			go ftClient.handlePacket(myPacket)
			// go doSomethingWithAPacket(myPacket)
			continue
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

// TODO: refactor to use some kind of "telemetry provider" interface instead of just packets/[]byte
type FormulaTelIngest struct {
	pb.CarMotionDataServiceClient
	capture bool
}

// handlePacket reads a packet header and calls Route on the remaining bytes
func (f *FormulaTelIngest) handlePacket(packet []byte) {
	buf := bytes.NewBuffer(packet)
	header := ReadBin[model.PacketHeader](buf)
	f.Route(header, buf)
}

// Route uses the [PacketType] of `header` to read the bytes from `reader` into the appropriate type.
// It generally calls makes an RPC call afterwards
func (f *FormulaTelIngest) Route(header *model.PacketHeader, data *bytes.Buffer) error {
	// TODO: get rid of this, it's just for capturing packets
	capture := data.Bytes()        // read the buffer; all the bytes - header
	packet := bytes.Clone(capture) // again all of the bytes - header
	fmt.Sprintf("%+v\n", header)
	switch header.PacketId {
	case model.EventPacket:
		event := ReadBin[model.EventData](bytes.NewBuffer(packet))
		fmt.Printf("%+v\n%+v\n", header, event)
	case model.CarMotionPacket:
		motion := ReadBin[model.CarMotionData](bytes.NewBuffer(packet))
		fmt.Printf("%+v\n%+v\n", header, motion)
		if f.CarMotionDataServiceClient != nil {
			_, err := f.CarMotionDataServiceClient.SendCarMotionData(context.TODO(), &pb.CarMotionData{
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
	}

	if f.capture {
		packetCapture, err := os.CreateTemp("captured_packets", fmt.Sprintf("%d_%d_%d", header.PacketId, header.SessionUID, time.Now().Nanosecond()))
		if err != nil {
			return err
		}
		defer packetCapture.Close()
		packetCapture.Write(capture)

	}

	return nil
}
