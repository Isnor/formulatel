package f123

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"time"
	"unsafe"

	pb "github.com/isnor/formulatel/internal/genproto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TODO: make this configurable
const maxPacketSize = 2048 // the largest packet is just 1460 bytes.

// F123PacketBuffer is an attempt to make the flow of "read 29 bytes", "determine type", "decode body"
// more organized and testable. It doesn't do that very well. To use this:
// 1 - call [UnpackHeader] to unpack the header of the packet
// 2 - use the [PacketHeader.PacketId] field to determine the type T
// 3 - call [f123.UnpackBody[T]] to unpack the body of the packet
type F123PacketBuffer struct {
	*bytes.Buffer
	header *PacketHeader
}

// NewF123PacketBuffer wraps packet in a [bytes.Buffer] and should therefore not be used after calling
// this function.
func NewF123PacketBuffer(packet []byte) *F123PacketBuffer {
	return &F123PacketBuffer{
		Buffer: bytes.NewBuffer(packet),
	}
}

// UnpackHeader returns the header of the buffer of `b`. The first successful call is stored.
// This function should be called before reading any data from the underlying buffer of `b`.
func (b *F123PacketBuffer) UnpackHeader() (*PacketHeader, error) {
	if b.header != nil {
		return b.header, nil
	}
	header := &PacketHeader{}

	if err := binary.Read(b, binary.LittleEndian, header); err != nil {
		slog.Error("error reading header", "error", err, "unread", b.Len())
		return nil, err
	}
	b.header = header
	return b.header, nil
}

// UnpackBody reads the body of an f123 packet _whose header has already been read from the `packet` buffer_ into
// the specified type and returns a pointer to that data. T must correspond to the data being read, i.e.
// it should be a struct whose fields are laid out such that `binary.Read` can unpack
// from `reader` as LittleEndian binary data. Errors encountered during `binary.Read` are returned.
func UnpackBody[T any](packet *F123PacketBuffer) (*T, error) {
	if packet.header == nil { // TODO: maybe start reading 29 bytes in if the header is nil?
		return nil, errors.New("ReadHeader must be called on [packet] before ReadBody can use it")
	}
	body := new(T)
	slog.Debug("received body packet", "remaining_bytes", packet.Len(), "struct_size", unsafe.Sizeof(*body))
	if err := binary.Read(packet, binary.LittleEndian, body); err != nil {
		slog.Error("failed reading packet body", "error", err, "packet_size", packet.Cap(), "struct_size", unsafe.Sizeof(*body))
		return body, err
	}
	slog.Debug("read body", "body", *body)
	return body, nil
}

// F123PacketListener listens for packets on a UDP connection and puts them in PacketBuffer
type F123PacketListener struct {
	Server       net.PacketConn
	PacketBuffer chan<- []byte

	finishedWithError error
}

// Listen tries to read packets until `serverContext` is cancelled. If it is, Listen
// returns nil. If Listen returns a non-nil error, subsequent calls to Listen return
// the same error and no packets are read.
func (f *F123PacketListener) Listen(serverContext context.Context) error {
	if f.finishedWithError != nil {
		return f.finishedWithError
	}
	slog.InfoContext(serverContext, "starting formulatel f123 packet reader")

	// listen for packets
	for {
		select {
		case <-serverContext.Done():
			slog.InfoContext(serverContext, "formulatel f123 packet reader closed")
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
				if err != nil {
					slog.ErrorContext(serverContext, "error reading packet", "error", err)
				}
				f.PacketBuffer <- packet[:numRead]
				slog.DebugContext(serverContext, "formulatel f123 read a packet", "bytes_read", numRead)
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
	Packets                  <-chan []byte
	VehicleDataChannel       chan<- *pb.GameTelemetry // a channel for the vehicle data
	MotionDataChannel        chan<- *pb.GameTelemetry // a channel for motion data
	CurrentLapDataChannel    chan<- *pb.GameTelemetry // a channel for current lap data
	LapTimesDataChannel      chan<- *pb.GameTelemetry // a channel for lap times data
	ExtendedWheelDataChannel chan<- *pb.GameTelemetry // a channel for extended wheel data
	LatestLaps               *LatestLapData           // a cache of the index of each car's latest recorded lap in the session
	capture                  bool                     // TODO: remove, just for testing. when set, writes a file for every packet received

	timeAtLastCapture time.Time // used to "rate limit" the number of packets we capture
}

// Consume reads packets from a buffered channel until the channel is closed or the reader is shutdown
func (f *F123PacketTransformer) Consume(ctx context.Context) {
	slog.InfoContext(ctx, "f123 packet transformer starting consuming")
	for {
		select {
		case <-ctx.Done():
			slog.InfoContext(ctx, "f123 packet transformer stopping consuming")
			return
		case packet, ok := <-f.Packets:
			// Channel is closed (ok == false) when Run() exits
			if !ok {
				// TODO: we should probably flush the channel here, no?
				slog.InfoContext(ctx, "f123 packet channel closed, transformer stopping consume")
				return
			}
			f.handlePacket(ctx, packet)
		}
	}
}

// Route uses the [PacketType] of `header` to read the bytes from `reader` into the appropriate type.
// TODO: consider the signature here, it was hacked together initially
func (f *F123PacketTransformer) Route(ctx context.Context, header *PacketHeader, data *F123PacketBuffer) error {

	switch header.PacketId {
	case CarTelemetryPacket:
		telemetryArray, err := UnpackBody[[22]CarTelemetryData](data)
		if err != nil {
			return err
		}
		playerTelemetry := telemetryArray[header.PlayerCarIndex]
		slog.DebugContext(ctx, "read a car telemetry packet")

		telproto := &pb.GameTelemetry{
			Title:     pb.GameTitle_GAME_TITLE_F123,
			SessionId: fmt.Sprint(header.SessionUID),
			UserId:    fmt.Sprint(header.PlayerCarIndex),
			Timestamp: timestamppb.Now(),
			Data: &pb.GameTelemetry_VehicleData{
				VehicleData: f.normalizeVehicleData(&playerTelemetry),
			},
		}
		f.VehicleDataChannel <- telproto
	case CarMotionPacket:
		motionArray, err := UnpackBody[[22]CarMotionData](data)
		if err != nil {
			return err
		}
		playerMotion := motionArray[header.PlayerCarIndex]
		slog.DebugContext(ctx, "read a motion packet")

		motionProto := &pb.GameTelemetry{
			Title:     pb.GameTitle_GAME_TITLE_F123,
			SessionId: fmt.Sprint(header.SessionUID),
			UserId:    fmt.Sprint(header.PlayerCarIndex),
			Timestamp: timestamppb.Now(),
			Data: &pb.GameTelemetry_MotionData{
				MotionData: f.normalizeMotionData(&playerMotion),
			},
		}
		f.MotionDataChannel <- motionProto
	case LapDataPacket:
		// LapDataPacket is 22 bytes, read into LapData array
		lapArray, err := UnpackBody[[22]LapData](data)
		if err != nil {
			return err
		}
		playerLapData := lapArray[header.PlayerCarIndex]
		slog.DebugContext(ctx, "read a lap data packet")

		currentLapData := &pb.CurrentLapData{
			LapTime:           uint32(playerLapData.LastLapTimeInMS),
			Sector1Time:       uint32(playerLapData.Sector1TimeInMS),
			Sector2Time:       uint32(playerLapData.Sector2TimeInMS),
			DeltaToCarInFront: uint32(playerLapData.DeltaToCarInFrontInMS),
			DeltaToRaceLeader: uint32(playerLapData.DeltaToRaceLeaderInMS),
			LapDistance:       playerLapData.LapDistance,
			TotalDistance:     playerLapData.TotalDistance,
			LapNum:            uint32(playerLapData.CurrentLapNum),
			Sector:            uint32(playerLapData.Sector),
		}

		lapProto := &pb.GameTelemetry{
			Title:     pb.GameTitle_GAME_TITLE_F123,
			SessionId: fmt.Sprint(header.SessionUID),
			UserId:    fmt.Sprint(header.PlayerCarIndex),
			Timestamp: timestamppb.Now(),
			Data: &pb.GameTelemetry_CurrentLapData{
				CurrentLapData: currentLapData,
			},
		}
		f.CurrentLapDataChannel <- lapProto
	case SessionHistoryPacket:
		// SessionHistoryPacket contains historical lap data with complete sector times
		// This packet is sent at 20Hz cycling through cars (one car per packet).
		sessionHistoryPacket, err := UnpackBody[SessionHistoryData](data)
		if err != nil {
			return err
		}
		// TODO: ignoring non-player packets should be configurable
		// Ignore packets that don't come from the player's car.
		if sessionHistoryPacket.CurrentCarIdx == header.PlayerCarIndex {
			// this is a weird one, but what's happening is this session history packet comes in with
			// all of the completed laps and we're unpacking them to store in our lap_data table.
			lapTimesData := f.normalizeSessionHistoryData(header, sessionHistoryPacket)
			if len(lapTimesData) > 0 {
				slog.InfoContext(ctx, "read a session history packet for player car", "laps_read", len(lapTimesData))

				for _, lapTime := range lapTimesData {
					sessionHistoryProto := &pb.GameTelemetry{
						Title:     pb.GameTitle_GAME_TITLE_F123,
						SessionId: fmt.Sprint(header.SessionUID),
						UserId:    fmt.Sprint(header.PlayerCarIndex),
						Timestamp: timestamppb.Now(),
						Data: &pb.GameTelemetry_LapTimesData{
							LapTimesData: lapTime,
						},
					}
					f.LapTimesDataChannel <- sessionHistoryProto
				}
			}
		}
	case MotionExPacket:
		// MotionExPacket contains extended motion data for player car only
		playerExtWheels, err := UnpackBody[ExtendedMotionData](data)
		if err != nil {
			return err
		}
		slog.DebugContext(ctx, "read a motion ex packet for player car")

		extendedWheelData := f.normalizeMotionExData(playerExtWheels)

		motionExProto := &pb.GameTelemetry{
			Title:     pb.GameTitle_GAME_TITLE_F123,
			SessionId: fmt.Sprint(header.SessionUID),
			UserId:    fmt.Sprint(header.PlayerCarIndex),
			Timestamp: timestamppb.Now(),
			Data: &pb.GameTelemetry_WheelData{
				WheelData: extendedWheelData,
			},
		}
		f.ExtendedWheelDataChannel <- motionExProto
	}

	return nil
}

// handlePacket reads a packet header and calls Route on the remaining bytes. It also deals with capturing
// the packet if `capture` is enabled.
func (f *F123PacketTransformer) handlePacket(ctx context.Context, packet []byte) {
	var clone []byte
	if f.capture {
		clone = bytes.Clone(packet) // create a copy of packet to write to a file because we pass ownership of packet to a byte buffer; only for packet capture
		f.handleCapture(ctx, clone)
	}
	packetBuf := NewF123PacketBuffer(packet)
	header, err := packetBuf.UnpackHeader()
	if err != nil {
		slog.ErrorContext(ctx, "transformer failed unpacking header", "error", err)
	}
	f.Route(ctx, header, packetBuf)
}

// handleCapture handles the arbitrary conditional logic for writing/capturing packets for replay
// and testing.
func (f *F123PacketTransformer) handleCapture(ctx context.Context, packet []byte) {
	packetReader := &F123PacketBuffer{Buffer: bytes.NewBuffer(packet)}
	slog.DebugContext(ctx, "decoding packet for capture", "bytes", packetReader.Len())
	header, err := packetReader.UnpackHeader()
	if err != nil {
		slog.ErrorContext(ctx, "failed capturing packet")
		return
	}
	slog.DebugContext(ctx, "read header",
		// "header", *header,
		"remaining_bytes", packetReader.Len(),
	)

	// session History capture logic
	if header.PacketId == SessionHistoryPacket {
		sessionHistory, _ := UnpackBody[SessionHistoryData](packetReader)
		if sessionHistory == nil {
			return
		}
		slog.InfoContext(ctx, "read session history", "num_laps", sessionHistory.NumLaps, "car_index", sessionHistory.CurrentCarIdx)
		if sessionHistory.CurrentCarIdx == header.PlayerCarIndex && sessionHistory.NumLaps > 1 && time.Since(f.timeAtLastCapture) > time.Second {
			packetCapture, err := os.CreateTemp("captured_packets", fmt.Sprintf("%d_%d_%d", header.PacketId, header.SessionUID, time.Now().Nanosecond()))
			if err != nil {
				slog.ErrorContext(ctx, "failed writing capture", "error", err)
			}
			defer packetCapture.Close()
			numWrote, err := packetCapture.Write(packet)
			defer func() { f.timeAtLastCapture = time.Now() }()
			if err != nil {
				slog.ErrorContext(ctx, "failed writing capture", "error", err)
			} else {
				slog.InfoContext(ctx, "wrote a session history packet", "bytes_wrote", numWrote, "num_laps", sessionHistory.NumLaps, "laps", sessionHistory.LapHistoryData[:sessionHistory.NumLaps])
			}
		}
	}
}

// normalizeVehicleData maps F1 23 CarTelemetryData to protobuf VehicleData
func (f *F123PacketTransformer) normalizeVehicleData(
	telemetry *CarTelemetryData,
) *pb.VehicleData {
	tires := &pb.VehicleData_Tires{
		FrontLeft: &pb.TireData{
			BrakeTemperature:   uint32(telemetry.BrakesTemperature[WheelIndexFrontLeft]),
			InnerTemperature:   uint32(telemetry.TyresInnerTemperature[WheelIndexFrontLeft]),
			SurfaceTemperature: uint32(telemetry.TyresSurfaceTemperature[WheelIndexFrontLeft]),
			Pressure:           uint32(telemetry.TyresPressure[WheelIndexFrontLeft]),
		},
		FrontRight: &pb.TireData{
			BrakeTemperature:   uint32(telemetry.BrakesTemperature[WheelIndexFrontRight]),
			InnerTemperature:   uint32(telemetry.TyresInnerTemperature[WheelIndexFrontRight]),
			SurfaceTemperature: uint32(telemetry.TyresSurfaceTemperature[WheelIndexFrontRight]),
			Pressure:           uint32(telemetry.TyresPressure[WheelIndexFrontRight]),
		},
		BackLeft: &pb.TireData{
			BrakeTemperature:   uint32(telemetry.BrakesTemperature[WheelIndexRearLeft]),
			InnerTemperature:   uint32(telemetry.TyresInnerTemperature[WheelIndexRearLeft]),
			SurfaceTemperature: uint32(telemetry.TyresSurfaceTemperature[WheelIndexRearLeft]),
			Pressure:           uint32(telemetry.TyresPressure[WheelIndexRearLeft]),
		},
		BackRight: &pb.TireData{
			BrakeTemperature:   uint32(telemetry.BrakesTemperature[WheelIndexRearRight]),
			InnerTemperature:   uint32(telemetry.TyresInnerTemperature[WheelIndexRearRight]),
			SurfaceTemperature: uint32(telemetry.TyresSurfaceTemperature[WheelIndexRearRight]),
			Pressure:           uint32(telemetry.TyresPressure[WheelIndexRearRight]),
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

// normalizeSessionHistoryData unpacks the lap data from the SessionHistoryData field
func (f *F123PacketTransformer) normalizeSessionHistoryData(
	header *PacketHeader,
	sessionHistory *SessionHistoryData,
) []*pb.HistoricLapData {
	// this data comes with the current incomplete lap which we do not want for this channel.
	// It will always be the last entry, so we'll remove that from the list of laps
	// we're sending
	maxLapIndex := int(sessionHistory.NumLaps) - 1
	if maxLapIndex <= 0 {
		slog.Info("f123 packet transformer not sending current incomplete lap as historic lap")
		return nil
	}
	laps := make([]*pb.HistoricLapData, maxLapIndex)
	incomingLaps := sessionHistory.LapHistoryData
	// get latest index for this session, car
	sessionID, userID := fmt.Sprintf("%d", header.SessionUID), fmt.Sprintf("%d", header.PlayerCarIndex)
	latest := f.LatestLaps.Get(sessionID, userID)
	if latest >= maxLapIndex {
		slog.Info("f123 packet transformer not sending already-recorded historic lap", "latest_recorded", latest, "completed_laps_received", maxLapIndex)
		return nil
	}
	f.LatestLaps.Set(sessionID, userID, maxLapIndex)
	for i := range maxLapIndex {
		lap := incomingLaps[i]

		laps[i] = &pb.HistoricLapData{
			LapNum:       uint32(i),
			LapTime:      durationpb.New(time.Millisecond * time.Duration(lap.LapTimeInMS)),
			Sector1Time:  durationpb.New(time.Millisecond * time.Duration(lap.Sector1TimeInMS)),
			Sector2Time:  durationpb.New(time.Millisecond * time.Duration(lap.Sector2TimeInMS)),
			Sector3Time:  durationpb.New(time.Millisecond * time.Duration(lap.Sector3TimeInMS)),
			LapValid:     (lap.LapValidBitFlags & 0x01) != 0,
			Sector1Valid: (lap.LapValidBitFlags & 0x02) != 0,
			Sector2Valid: (lap.LapValidBitFlags & 0x04) != 0,
			Sector3Valid: (lap.LapValidBitFlags & 0x08) != 0,
		}
	}
	return laps
}

// normalizeMotionExData unpacks the wheel data from the MotionExPacket
func (f *F123PacketTransformer) normalizeMotionExData(
	motionEx *ExtendedMotionData,
) *pb.ExtendedFourWheelData {
	return &pb.ExtendedFourWheelData{
		BackLeft: &pb.ExtendedWheelData{
			WheelSpeed:         motionEx.WheelSpeed[WheelIndexRearLeft],
			VerticalForce:      motionEx.WheelVertForce[WheelIndexRearLeft],
			SlipAngle:          motionEx.WheelSlipAngle[WheelIndexRearLeft],
			SlipRatio:          motionEx.WheelSlipRatio[WheelIndexRearLeft],
			LateralForce:       motionEx.WheelLatForce[WheelIndexRearLeft],
			LongitudinalForce:  motionEx.WheelLonForce[WheelIndexRearLeft],
			SuspensionPosition: motionEx.SuspensionPosition[WheelIndexRearLeft],
			SuspensionVelocity: motionEx.SuspensionVelocity[WheelIndexRearLeft],
		},
		BackRight: &pb.ExtendedWheelData{
			WheelSpeed:         motionEx.WheelSpeed[WheelIndexRearRight],
			VerticalForce:      motionEx.WheelVertForce[WheelIndexRearRight],
			SlipAngle:          motionEx.WheelSlipAngle[WheelIndexRearRight],
			SlipRatio:          motionEx.WheelSlipRatio[WheelIndexRearRight],
			LateralForce:       motionEx.WheelLatForce[WheelIndexRearRight],
			LongitudinalForce:  motionEx.WheelLonForce[WheelIndexRearRight],
			SuspensionPosition: motionEx.SuspensionPosition[WheelIndexRearRight],
			SuspensionVelocity: motionEx.SuspensionVelocity[WheelIndexRearRight],
		},
		FrontLeft: &pb.ExtendedWheelData{
			WheelSpeed:         motionEx.WheelSpeed[WheelIndexFrontLeft],
			VerticalForce:      motionEx.WheelVertForce[WheelIndexFrontLeft],
			SlipAngle:          motionEx.WheelSlipAngle[WheelIndexFrontLeft],
			SlipRatio:          motionEx.WheelSlipRatio[WheelIndexFrontLeft],
			LateralForce:       motionEx.WheelLatForce[WheelIndexFrontLeft],
			LongitudinalForce:  motionEx.WheelLonForce[WheelIndexFrontLeft],
			SuspensionPosition: motionEx.SuspensionPosition[WheelIndexFrontLeft],
			SuspensionVelocity: motionEx.SuspensionVelocity[WheelIndexFrontLeft],
		},
		FrontRight: &pb.ExtendedWheelData{
			WheelSpeed:         motionEx.WheelSpeed[WheelIndexFrontRight],
			VerticalForce:      motionEx.WheelVertForce[WheelIndexFrontRight],
			SlipAngle:          motionEx.WheelSlipAngle[WheelIndexFrontRight],
			SlipRatio:          motionEx.WheelSlipRatio[WheelIndexFrontRight],
			LateralForce:       motionEx.WheelLatForce[WheelIndexFrontRight],
			LongitudinalForce:  motionEx.WheelLonForce[WheelIndexFrontRight],
			SuspensionPosition: motionEx.SuspensionPosition[WheelIndexFrontRight],
			SuspensionVelocity: motionEx.SuspensionVelocity[WheelIndexFrontRight],
		},
	}
}

// F123IngestConfig exposes config options for the underlying PacketReader and PacketTransformer.
// It also serves a container for the data channels that the Transformer writes to
type F123IngestConfig struct {
	MaxPacketsBuffered uint   `split_words:"true" default:"1000"` // size of the buffered channel of packets
	CapturePackets     bool   `split_words:"true" default:"false"`
	UDPPort            uint16 `envconfig:"UDP_PORT" default:"27543"`

	VehicleDataChannel       chan *pb.GameTelemetry `envconfig:"-"`
	MotionDataChannel        chan *pb.GameTelemetry `envconfig:"-"`
	CurrentLapDataChannel    chan *pb.GameTelemetry `envconfig:"-"`
	LapTimesDataChannel      chan *pb.GameTelemetry `envconfig:"-"`
	ExtendedWheelDataChannel chan *pb.GameTelemetry `envconfig:"-"`
}

// F123Ingest is simply a convenient container
type F123Ingest struct {
	*F123PacketListener
	*F123PacketTransformer
}

// NewF123Ingest is a convenience function that uses `cfg` to setup a PacketReader and PacketTransformer
func NewF123Ingest(
	cfg F123IngestConfig,
	conn net.PacketConn,
) *F123Ingest {
	buffer := make(chan []byte, cfg.MaxPacketsBuffered)
	packetReader := &F123PacketListener{
		Server:       conn,
		PacketBuffer: buffer,
	}
	transformer := &F123PacketTransformer{
		Packets:                  buffer,                    // read and unpack F123 packets, placing them in a data-specific channel
		VehicleDataChannel:       cfg.VehicleDataChannel,    // write vehicle packets as their protobuf representation here
		MotionDataChannel:        cfg.MotionDataChannel,     // write motion packets as their protobuf representation here
		CurrentLapDataChannel:    cfg.CurrentLapDataChannel, // write current lap times packets as their protobuf representation here
		LapTimesDataChannel:      cfg.LapTimesDataChannel,   // write historic lap data packets as their protobuf representation here
		ExtendedWheelDataChannel: cfg.ExtendedWheelDataChannel,
		LatestLaps:               NewLatestLapData(),
		capture:                  cfg.CapturePackets,
	}
	return &F123Ingest{
		F123PacketListener:    packetReader,
		F123PacketTransformer: transformer,
	}
}
