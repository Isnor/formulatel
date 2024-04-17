package formulatel

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
	"sync/atomic"
	"time"

	pb "github.com/isnor/formulatel/internal/genproto"

	"github.com/isnor/formulatel/model"
)

const MaxPacketSize = 2048 // the largest packet is just 1460 bytes.

// this is fairly EA/codemasters F1-specific
func ReadBin[T any](reader io.Reader) *T {
	x := new(T)
	binary.Read(reader, binary.LittleEndian, x)
	return x
}

type F123FormulaTelIngest struct {
	Server       net.PacketConn
	Shutdown     *atomic.Bool
	Cancel       context.CancelFunc
	PacketBuffer chan<- []byte
}

func (f *F123FormulaTelIngest) Run(serverContext context.Context) {
	slog.InfoContext(serverContext, "starting formulatel server")

	// listen for packets
	for !f.Shutdown.Load() {
		select {
		// this is our "graceful shutdown" attempt
		case <-serverContext.Done():
			slog.InfoContext(serverContext, "closing server")
			f.Shutdown.Store(true)
			close(f.PacketBuffer)
		default:
			// read a packet of data up to MaxPacketSize bytes from a UDP connection
			// TODO: there's probably a better way to handle this deadline / avoid blocking on ReadFrom, this is just the first
			if err := f.Server.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
				slog.Error("error reading from UDP connection", "error", err)
			}
			var packet []byte = make([]byte, MaxPacketSize)
			numRead, _, err := f.Server.ReadFrom(packet)

			// per ReadFrom doc, we should check the number of bytes read before looking at the error.
			if numRead > 0 {
				// all the server does is read packet by packet into a channel. The server needs to create
				// child routines to read the packets and handle them with the client
				f.PacketBuffer <- packet
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

// this is a formulatel client that unpacks F123 packets and puts them in a channel
// specific to the type of telemetry in the packet
type F123PacketReader struct {
	Shutdown           *atomic.Bool
	Packets            <-chan []byte
	VehicleDataChannel chan<- *pb.GameTelemetry // a channel for the vehicle data
	capture            bool                     // TODO: remove, just for testing. when set, writes a file for every packet received
}

// Consume reads packets from a buffered channel until the channel is closed or the reader is shutdown
func (f *F123PacketReader) Consume(ctx context.Context) {
	slog.InfoContext(ctx, "starting reader")
	for packet := range f.Packets {
		f.handlePacket(ctx, packet)
		if f.Shutdown.Load() {
			break
		}
	}
	close(f.VehicleDataChannel)
	slog.InfoContext(ctx, "closing reader")
}

// handlePacket reads a packet header and calls Route on the remaining bytes
func (f *F123PacketReader) handlePacket(ctx context.Context, packet []byte) {
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
func (f *F123PacketReader) Route(ctx context.Context, header *model.PacketHeader, data *bytes.Buffer) error {

	// TODO: create a child context and add tracing
	// todoContext := ctx
	switch header.PacketId {
	case model.CarMotionPacket:
		motionArray := ReadBin[[22]model.CarMotionData](data)
		motion := motionArray[header.PlayerCarIndex]
		fmt.Printf("%+v\n%+v\n", header, motion)
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
	case model.CarTelemetryPacket:
		telemetryArray := ReadBin[[22]model.CarTelemetryData](data)
		playerTelemetry := telemetryArray[header.PlayerCarIndex]

		println(playerTelemetry.Speed)

		telproto := &pb.GameTelemetry{
			Title:     pb.GameTitle_GAME_TITLE_F123,
			SessionId: fmt.Sprint(header.SessionUID),
			UserId:    fmt.Sprint(header.PlayerCarIndex),
			Data: &pb.GameTelemetry_VehicleData{
				VehicleData: &pb.VehicleData{
					Speed: uint32(playerTelemetry.Speed),
				},
			},
		}
		f.VehicleDataChannel <- telproto
	}

	return nil
}
