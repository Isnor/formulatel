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
	"sync"
	"time"

	pb "github.com/isnor/formulatel/internal/genproto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TODO: make this configurable
const maxPacketSize = 2048 // the largest packet is just 1460 bytes.

// this is a map of the (session, user) to the index of the latest full lap that ingest has received and sent
type LatestLapData struct {
	lock sync.RWMutex
	data map[string]int
}

func NewLatestLapData() *LatestLapData {
	return &LatestLapData{
		data: make(map[string]int),
	}
}

func (l *LatestLapData) Set(sessionID, userID string, lapNum int) int {
	l.lock.Lock()
	defer l.lock.Unlock()
	key := fmt.Sprintf("%s.%s", sessionID, userID)
	if latestLapNum := l.data[key]; lapNum > latestLapNum {
		l.data[key] = lapNum
	}
	return l.data[key]
}

func (l *LatestLapData) Get(sessionID, userID string) int {
	l.lock.RLock()
	defer l.lock.RUnlock()
	key := fmt.Sprintf("%s.%s", sessionID, userID)
	return l.data[key]
}

// this is fairly EA/codemasters F1-specific
// ReadBin uses `reader` to unpack some binary data into a new struct of type T, and
// returns a pointer to that struct. T must correspond to the data being read, i.e.
// it should be a struct whose fields are laid out such that `binary.Read` can unpack
// from `reader` as LittleEndian binary data.
func ReadBin[T any](reader io.Reader) *T {
	x := new(T)
	// TODO: why are we throwing this error away?
	binary.Read(reader, binary.LittleEndian, x)
	return x
}

// F123PacketReader listens for packets on a UDP connection and puts them in PacketBuffer
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
				f.PacketBuffer <- packet
				slog.DebugContext(serverContext, "formulatel f123 read a packet")
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
	Packets               <-chan []byte
	VehicleDataChannel    chan<- *pb.GameTelemetry // a channel for the vehicle data
	MotionDataChannel     chan<- *pb.GameTelemetry // a channel for motion data
	CurrentLapDataChannel chan<- *pb.GameTelemetry // a channel for current lap data
	LapTimesDataChannel   chan<- *pb.GameTelemetry // a channel for lap times data
	LatestLaps            *LatestLapData           // a cache of the index of each car's latest recorded lap in the session
	capture               bool                     // TODO: remove, just for testing. when set, writes a file for every packet received
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

// normalizeSessionHistoryData unpacks the lap data from the SessionHistoryData field
func (f *F123PacketTransformer) normalizeSessionHistoryData(
	header *PacketHeader,
	sessionHistory *SessionHistoryData,
) []*pb.HistoricLapData {
	// this data comes with the current incomplete lap which we do not want for this channel.
	// Presumably it will always be the last entry, so we'll remove that from the list of laps
	// we're sending
	maxLapIndex := int(sessionHistory.NumLaps) - 1
	if maxLapIndex <= 0 {
		slog.Info("f123 packet transformer not sending current incomplete lap as historic lap", "laps", sessionHistory.LapHistoryData)
		return nil
	}
	laps := make([]*pb.HistoricLapData, maxLapIndex)
	incomingLaps := sessionHistory.LapHistoryData
	// get latest index for this session, car
	sessionID, userID := fmt.Sprintf("%d", header.SessionUID), fmt.Sprintf("%d", header.PlayerCarIndex)
	latest := f.LatestLaps.Get(sessionID, userID)
	if latest >= maxLapIndex {
		slog.Info("f123 packet transformer not sending already-recorded historic lap", "latest_recorded", latest, "max_received", maxLapIndex)
		return nil
	}
	f.LatestLaps.Set(sessionID, userID, maxLapIndex)
	for i := latest; i < maxLapIndex; i++ {
		lap := incomingLaps[i]

		laps = append(laps, &pb.HistoricLapData{
			LapNum:       uint32(i),
			LapTime:      durationpb.New(time.Millisecond * time.Duration(lap.LapTimeInMS)),
			Sector1Time:  durationpb.New(time.Millisecond * time.Duration(lap.Sector1TimeInMS)),
			Sector2Time:  durationpb.New(time.Millisecond * time.Duration(lap.Sector2TimeInMS)),
			Sector3Time:  durationpb.New(time.Millisecond * time.Duration(lap.Sector3TimeInMS)),
			LapValid:     lap.LapValidBitFlags&0x01 == 0,
			Sector1Valid: lap.LapValidBitFlags&0x02 == 0,
			Sector2Valid: lap.LapValidBitFlags&0x04 == 0,
			Sector3Valid: lap.LapValidBitFlags&0x08 == 0,
		})
	}
	return laps
}

// Route uses the [PacketType] of `header` to read the bytes from `reader` into the appropriate type.
// TODO: consider the signature here, it was hacked together initially
func (f *F123PacketTransformer) Route(ctx context.Context, header *PacketHeader, data *bytes.Buffer) error {

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
		lapTimesData := &pb.CurrentLapData{
			LapTime:            uint32(playerLapData.LastLapTimeInMS),
			Sector1Time:        uint32(playerLapData.Sector1TimeInMS),
			Sector2Time:        uint32(playerLapData.Sector2TimeInMS),
			DeltaToCarInFront:  uint32(playerLapData.DeltaToCarInFrontInMS),
			DeltaToRaceLeader:  uint32(playerLapData.DeltaToRaceLeaderInMS),
			LapDistance:        playerLapData.LapDistance,
			TotalDistance:      playerLapData.TotalDistance,
			CarPosition:        uint32(playerLapData.CarPosition),
			GridPosition:       uint32(playerLapData.GridPosition),
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
			Data: &pb.GameTelemetry_CurrentLapData{
				CurrentLapData: lapTimesData,
			},
		}
		f.CurrentLapDataChannel <- lapProto
	case SessionHistoryPacket:
		// SessionHistoryPacket contains historical lap data with complete sector times
		// This packet is sent at 20Hz cycling through cars (one car per packet).
		sessionHistoryPacket := ReadBin[SessionHistoryData](data)
		// TODO: ignoring non-player packets should be configurable
		// Ignore packets that don't come from the player's car.
		if sessionHistoryPacket.CurrentCarIdx == header.PlayerCarIndex {
			// this is a weird one, but what's happening is this session history packet comes in with
			// all of the completed laps and we're unpacking them to store in our lap_data table.
			lapTimesData := f.normalizeSessionHistoryData(header, sessionHistoryPacket)
			if len(lapTimesData) > 0 {
				slog.InfoContext(ctx, "read a session history packet for player car", "laps_read", len(lapTimesData))
			}
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

	return nil
}
