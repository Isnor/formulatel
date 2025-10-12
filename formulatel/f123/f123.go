package f123

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
	"time"

	pb "github.com/isnor/formulatel/internal/genproto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const maxPacketSize = 2048 // the largest packet is just 1460 bytes.

// this is fairly EA/codemasters F1-specific
func ReadBin[T any](reader io.Reader) *T {
	x := new(T)
	binary.Read(reader, binary.LittleEndian, x)
	return x
}

type F123FormulaTelIngest struct {
	Server       net.PacketConn
	Cancel       context.CancelFunc
	PacketBuffer chan<- []byte
}

func (f *F123FormulaTelIngest) Run(serverContext context.Context) {
	slog.InfoContext(serverContext, "starting formulatel ingest")

	// listen for packets
	for {
		select {
		// this is our "graceful shutdown" attempt
		case <-serverContext.Done():
			slog.InfoContext(serverContext, "formulatel ingest closed")
			return
		default:
			// read a packet of data up to MaxPacketSize bytes from a UDP connection
			// TODO: there's probably a better way to handle this deadline / avoid blocking on ReadFrom, this is just the first
			if err := f.Server.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
				slog.Error("error reading from UDP connection", "error", err)
			}
			var packet []byte = make([]byte, maxPacketSize)
			numRead, _, err := f.Server.ReadFrom(packet)

			// per ReadFrom doc, we should check the number of bytes read before looking at the error.
			if numRead > 0 {
				// all the server does is read packet by packet into a channel. The server needs to create
				// child routines to read the packets and handle them with the client
				f.PacketBuffer <- packet
				slog.DebugContext(serverContext, "f123 wrote a packet")
				continue
			} else {
				time.Sleep(500 * time.Millisecond) // we didn't receive any bytes, wait for a bit
			}

			if err != nil && !errors.Is(err, os.ErrDeadlineExceeded) {
				slog.ErrorContext(serverContext, "failed reading packets:", "error", err)
				f.Cancel() // cancels the server context and effectively shuts down
			}
		}
	}
}

// this is a game-specific implementation detail that unpacks F123 packets and puts them in a channel
// specific to the type of telemetry in the packet
type F123PacketReader struct {
	Packets            <-chan []byte
	VehicleDataChannel chan<- *pb.GameTelemetry // a channel for the vehicle data
	MotionDataChannel  chan<- *pb.GameTelemetry // a channel for motion data
	capture            bool                     // TODO: remove, just for testing. when set, writes a file for every packet received
}

// Consume reads packets from a buffered channel until the channel is closed or the reader is shutdown
func (f *F123PacketReader) Consume(ctx context.Context) {
	slog.InfoContext(ctx, "f123 packet reader starting consuming")
	for {
		select {
		case <-ctx.Done():
			slog.InfoContext(ctx, "f123 packet reader stopping consuming")
			return
		case packet := <-f.Packets:
			f.handlePacket(ctx, packet)
		}
	}
}

// handlePacket reads a packet header and calls Route on the remaining bytes
func (f *F123PacketReader) handlePacket(ctx context.Context, packet []byte) {
	var clone []byte
	if f.capture {
		clone = bytes.Clone(packet) // create a copy of packet to write to a file because we pass ownership of packet to a byte buffer; only for packet capture
	}
	buf := bytes.NewBuffer(packet)
	header := ReadBin[PacketHeader](buf)
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
// TODO: consider the signature here, it was hacked together initially
func (f *F123PacketReader) Route(ctx context.Context, header *PacketHeader, data *bytes.Buffer) error {

	// TODO: create a child context and add tracing
	// todoContext := ctx
	switch header.PacketId {
	case CarMotionPacket:
		motionArray := ReadBin[[22]CarMotionData](data)
		motion := motionArray[header.PlayerCarIndex]
		slog.DebugContext(ctx, "got a motion packet", "data", fmt.Sprintf("%#v", motion))
		// TODO: need to add the protobuf for MotionData and/or figure out how we're going to handle this.
		t := &pb.GameTelemetry{
			Title: pb.GameTitle_GAME_TITLE_F123,
			Data: &pb.GameTelemetry_VehicleData{
				VehicleData: &pb.VehicleData{
					// TODO: very much incorrect
					Steering: motion.WorldPositionX,
				},
			},
		}
		f.MotionDataChannel <- t
		// if f.CarMotionDataServiceClient != nil {
		// 	_, err := f.CarMotionDataServiceClient.SendCarMotionData(todoContext, &f123.CarMotionData{
		// 		WorldPositionX:     motion.WorldPositionX,
		// 		WorldPositionY:     motion.WorldPositionY,
		// 		WorldPositionZ:     motion.WorldPositionZ,
		// 		WorldVelocityX:     motion.WorldVelocityX,
		// 		WorldVelocityY:     motion.WorldVelocityY,
		// 		WorldVelocityZ:     motion.WorldVelocityZ,
		// 		WorldForwardDirX:   int32(motion.WorldForwardDirX),
		// 		WorldForwardDirY:   int32(motion.WorldForwardDirY),
		// 		WorldForwardDirZ:   int32(motion.WorldForwardDirZ),
		// 		WorldRightDirX:     int32(motion.WorldRightDirX),
		// 		WorldRightDirY:     int32(motion.WorldRightDirY),
		// 		WorldRightDirZ:     int32(motion.WorldRightDirZ),
		// 		GForceLateral:      motion.GForceLateral,
		// 		GForceLongitudinal: motion.GForceLongitudinal,
		// 		GForceVertical:     motion.GForceVertical,
		// 		Yaw:                motion.Yaw,
		// 		Pitch:              motion.Pitch,
		// 		Roll:               motion.Roll,
		// 	})
		// 	if err != nil {
		// 		fmt.Println(fmt.Errorf("oh no, an error %s", err))
		// 	}
		// }
	case CarTelemetryPacket:
		telemetryArray := ReadBin[[22]CarTelemetryData](data)
		playerTelemetry := telemetryArray[header.PlayerCarIndex]
		slog.DebugContext(ctx, "read a car telemetry packet")

		telproto := &pb.GameTelemetry{
			Title:     pb.GameTitle_GAME_TITLE_F123,
			SessionId: fmt.Sprint(header.SessionUID),
			UserId:    fmt.Sprint(header.PlayerCarIndex),
			Timestamp: timestamppb.Now(),
			Data: &pb.GameTelemetry_VehicleData{
				VehicleData: &pb.VehicleData{
					Speed:             uint32(playerTelemetry.Speed),
					Rpm:               uint32(playerTelemetry.EngineRPM),
					Throttle:          playerTelemetry.Throttle,
					Break:             playerTelemetry.Brake,
					Steering:          playerTelemetry.Steer,
					Gear:              int32(playerTelemetry.Gear),
					EngineTemperature: uint32(playerTelemetry.EngineTemperature),
					// TODO: tires are a bit more complex because they aren't a 1:1 mapping from f123
					// Tires: ,
				},
			},
		}
		f.VehicleDataChannel <- telproto
	}

	return nil
}
