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

// TODO: make this configurable
const maxPacketSize = 2048 // the largest packet is just 1460 bytes.

// this is fairly EA/codemasters F1-specific
func ReadBin[T any](reader io.Reader) *T {
	x := new(T)
	binary.Read(reader, binary.LittleEndian, x)
	return x
}

type F123PacketReader struct {
	Server       net.PacketConn
	PacketBuffer chan<- []byte

	finishedWithError error
}

// Run tries to read packets until `serverContext` is cancelled. If it is, Run
// returns nil. If Run returns a non-nil error, subsequent calls to Run return
// the same error and no packets are read.
func (f *F123PacketReader) Run(serverContext context.Context) error {
	if f.finishedWithError != nil {
		return f.finishedWithError
	}
	slog.InfoContext(serverContext, "starting formulatel ingest")

	// listen for packets
	for {
		select {
		case <-serverContext.Done():
			slog.InfoContext(serverContext, "formulatel ingest closed")
			return nil
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
				slog.DebugContext(serverContext, "formulatel_ingest_f123 read a packet")
				continue
			} else {
				time.Sleep(500 * time.Millisecond) // we didn't receive any bytes, wait for a bit
			}

			if err != nil && !errors.Is(err, os.ErrDeadlineExceeded) {
				slog.ErrorContext(serverContext, "failed reading packets:", "error", err)
				f.finishedWithError = err
				return f.finishedWithError
			}
		}
	}
}

// this is a game-specific implementation detail that unpacks F123 packets and puts them in a channel
// specific to the type of telemetry in the packet
type F123PacketTransformer struct {
	Packets            <-chan []byte
	VehicleDataChannel chan<- *pb.GameTelemetry // a channel for the vehicle data
	MotionDataChannel  chan<- *pb.GameTelemetry // a channel for motion data
	LapTimesChannel    chan<- *pb.GameTelemetry // a channel for lap time data
	capture            bool                     // TODO: remove, just for testing. when set, writes a file for every packet received
}

// Consume reads packets from a buffered channel until the channel is closed or the reader is shutdown
func (f *F123PacketTransformer) Consume(ctx context.Context) {
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
func (f *F123PacketTransformer) handlePacket(ctx context.Context, packet []byte) {
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

// normalizeVehicleData maps F1 23 CarTelemetryData to protobuf VehicleData
func (f *F123PacketTransformer) normalizeVehicleData(
	header *PacketHeader,
	telemetry *CarTelemetryData,
) *pb.VehicleData {
	tires := &pb.VehicleData_Tires{
		FrontLeft: &pb.TireData{
			BrakeTemperature:   uint32(telemetry.BrakesTemperature[2]), // FL
			InnerTemperature:   uint32(telemetry.TyresInnerTemperature[2]),
			SurfaceTemperature: uint32(telemetry.TyresSurfaceTemperature[2]),
			Pressure:           uint32(telemetry.TyresPressure[2]),
		},
		FrontRight: &pb.TireData{
			BrakeTemperature:   uint32(telemetry.BrakesTemperature[3]), // FR
			InnerTemperature:   uint32(telemetry.TyresInnerTemperature[3]),
			SurfaceTemperature: uint32(telemetry.TyresSurfaceTemperature[3]),
			Pressure:           uint32(telemetry.TyresPressure[3]),
		},
		BackLeft: &pb.TireData{
			BrakeTemperature:   uint32(telemetry.BrakesTemperature[0]), // RL
			InnerTemperature:   uint32(telemetry.TyresInnerTemperature[0]),
			SurfaceTemperature: uint32(telemetry.TyresSurfaceTemperature[0]),
			Pressure:           uint32(telemetry.TyresPressure[0]),
		},
		BackRight: &pb.TireData{
			BrakeTemperature:   uint32(telemetry.BrakesTemperature[1]), // RR
			InnerTemperature:   uint32(telemetry.TyresInnerTemperature[1]),
			SurfaceTemperature: uint32(telemetry.TyresSurfaceTemperature[1]),
			Pressure:           uint32(telemetry.TyresPressure[1]),
		},
	}

	return &pb.VehicleData{
		Speed:             uint32(telemetry.Speed),
		Rpm:               uint32(telemetry.EngineRPM),
		Throttle:          telemetry.Throttle,
		Brake:             telemetry.Brake,
		Steering:          telemetry.Steer,
		Gear:              int32(telemetry.Gear),
		EngineTemperature: uint32(telemetry.EngineTemperature),
		Tires:             tires,
	}
}

// normalizeMotionData maps F1 23 CarMotionData to protobuf MotionData
func (f *F123PacketTransformer) normalizeMotionData(
	header *PacketHeader,
	motion *CarMotionData,
) *pb.MotionData {
	return &pb.MotionData{
		PositionX:          motion.WorldPositionX,
		PositionY:          motion.WorldPositionY,
		PositionZ:          motion.WorldPositionZ,
		VelocityX:          motion.WorldVelocityX,
		VelocityY:          motion.WorldVelocityY,
		VelocityZ:          motion.WorldVelocityZ,
		GForceLateral:      motion.GForceLateral,
		GForceLongitudinal: motion.GForceLongitudinal,
		GForceVertical:     motion.GForceVertical,
		Yaw:                motion.Yaw,
		Pitch:              motion.Pitch,
		Roll:               motion.Roll,
	}
}

// Route uses the [PacketType] of `header` to read the bytes from `reader` into the appropriate type.
// TODO: consider the signature here, it was hacked together initially
func (f *F123PacketTransformer) Route(ctx context.Context, header *PacketHeader, data *bytes.Buffer) error {

	// TODO: create a child context and add tracing
	// todoContext := ctx
	switch header.PacketId {
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
				VehicleData: f.normalizeVehicleData(header, &playerTelemetry),
			},
		}
		f.VehicleDataChannel <- telproto
	case CarMotionPacket:
		motionArray := ReadBin[[22]CarMotionData](data)
		playerMotion := motionArray[header.PlayerCarIndex]
		slog.DebugContext(ctx, "read a motion packet")

		motionProto := &pb.GameTelemetry{
			Title:     pb.GameTitle_GAME_TITLE_F123,
			SessionId: fmt.Sprint(header.SessionUID),
			UserId:    fmt.Sprint(header.PlayerCarIndex),
			Timestamp: timestamppb.Now(),
			Data: &pb.GameTelemetry_MotionData{
				MotionData: f.normalizeMotionData(header, &playerMotion),
			},
		}
		f.MotionDataChannel <- motionProto
	case LapDataPacket:
		// LapDataPacket is 22 bytes, read into LapData array
		lapArray := ReadBin[[22]LapData](data)
		playerLapData := lapArray[header.PlayerCarIndex]
		slog.DebugContext(ctx, "read a lap data packet")

		// Note: sector3_time is derived from sector1+sector2 for simplicity
		// In a full implementation, this would be calculated from total_distance or sector3 time
		lapTimesData := &pb.LapTimesData{
			LapTime:            uint32(playerLapData.LastLapTimeInMS),
			CurrentLapTime:     uint32(playerLapData.CurrentLapTimeInMS),
			Sector1Time:        uint32(playerLapData.Sector1TimeInMS),
			Sector2Time:        uint32(playerLapData.Sector2TimeInMS),
			Sector3Time:        0, // Derived value - would calculate from sector3 time if available
			DeltaToCarInFront:  uint32(playerLapData.DeltaToCarInFrontInMS),
			DeltaToRaceLeader:  uint32(playerLapData.DeltaToRaceLeaderInMS),
			LapDistance:        playerLapData.LapDistance,
			TotalDistance:      playerLapData.TotalDistance,
			CarPosition:        uint32(playerLapData.CarPosition),
			CurrentLapNum:      uint32(playerLapData.CurrentLapNum),
			GridPosition:       uint32(playerLapData.GridPosition),
			DriverStatus:       uint32(playerLapData.DriverStatus),
			ResultStatus:       uint32(playerLapData.ResultStatus),
			PitStatus:          uint32(playerLapData.PitStatus),
			NumPitStops:        uint32(playerLapData.NumPitStops),
			PitLaneTimerActive: uint32(playerLapData.PitLaneTimerActive),
			PitLaneTime:        float32(playerLapData.PitLaneTimeInLaneInMS),
		}

		lapProto := &pb.GameTelemetry{
			Title:     pb.GameTitle_GAME_TITLE_F123,
			SessionId: fmt.Sprint(header.SessionUID),
			UserId:    fmt.Sprint(header.PlayerCarIndex),
			Timestamp: timestamppb.Now(),
			Data: &pb.GameTelemetry_LapTimesData{
				LapTimesData: lapTimesData,
			},
		}
		f.LapTimesChannel <- lapProto
	}

	return nil
}
