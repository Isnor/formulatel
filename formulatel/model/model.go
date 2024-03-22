package model

// this file contains the spec for the EA/codemasters F1 23 telemetry packets, converted into Go
// From the spec document (23):
// DISCLAIMER: “This information is being provided under license from EA for reference purposes
// 	only and we do not make any representations or warranties about the accuracy or reliability of
// 	the information for any specific purpose.”

// PacketType represents the type of packet being sent.
type PacketType uint8

const (
	// CarMotionPacket contains all motion data for player’s car – only sent while player is in control.
	CarMotionPacket PacketType = iota

	// SessionPacket contains data about the session – track, time left.
	SessionPacket

	// LapDataPacket contains data about all the lap times of cars in the session.
	LapDataPacket

	// EventPacket contains various notable events that happen during a session.
	EventPacket

	// ParticipantsPacket represents a list of participants in the session, mostly relevant for multiplayer.
	ParticipantsPacket

	// CarSetupsPacket represents a packet detailing car setups for cars in the race.
	CarSetupsPacket

	// CarTelemetryPacket represents telemetry data for all cars.
	CarTelemetryPacket

	// CarStatusPacket represents status data for all cars.
	CarStatusPacket

	// FinalClassificationPacket represents final classification confirmation at the end of a race.
	FinalClassificationPacket

	// LobbyInfoPacket represents information about players in a multiplayer lobby.
	LobbyInfoPacket

	// CarDamagePacket represents damage status for all cars.
	CarDamagePacket

	// SessionHistoryPacket represents lap and tyre data for session.
	SessionHistoryPacket

	// TyreSetsPacket represents extended tyre set data.
	TyreSetsPacket

	// MotionExPacket represents extended motion data for player car.
	MotionExPacket
)

// this data is coming in little-endian packed format, so _do not_ change the field types or order.
type PacketHeader struct {
	PacketFormat            uint16     // 2023
	GameYear                uint8      // Game year - last two digits e.g. 23
	GameMajorVersion        uint8      // Game major version - "X.00"
	GameMinorVersion        uint8      // Game minor version - "1.XX"
	PacketVersion           uint8      // Version of this packet type, all start from 1
	PacketId                PacketType // Identifier for the packet type, see above PacketType constants
	SessionUID              uint64     // Unique identifier for the session
	SessionTime             float32    // Session timestamp
	FrameIdentifier         uint32     // Identifier for the frame the data was retrieved on
	OverallFrameIdentifier  uint32     // Overall identifier for the frame the data was retrieved on, doesn't go back after flashbacks
	PlayerCarIndex          uint8      // Index of player's car in the array
	SecondaryPlayerCarIndex uint8      // Index of secondary player's car in the array (splitscreen) 255 if no second player
}

// The packet spec is a bit different than the packet structs here because we need to read the header data out first to know what
// kind of data to read next and how `binary.Read` reads data into the struct. Alternatively we could reset the reader to index 0,
// but then we'd have to read the same 29 bytes again for every packet!
type EventData struct {
	// PacketHeader // the header will be read off the wire first
	EventStringCode [4]uint8
	// eventStringCode string // I thought this would actually make sense because we can't type the above as a string and have it read properly
	// 	I think maybe because the structs need to be a fixed size, and string is variable?
}

// This is the largest packet that is sent at the rate specified in the menu, and is therefore the largest consumer of bandwidth
// It is 1352 bytes and can be sent at a frequency up to 120Hz -> 120*1352= 162240 bytes/second, or ~159k/s
type CarTelemetryData struct {
	Speed                   uint16     // Speed of car in kilometres per hour
	Throttle                float32    // Amount of throttle applied (0.0 to 1.0)
	Steer                   float32    // Steering (-1.0 (full lock left) to 1.0 (full lock right))
	Brake                   float32    // Amount of brake applied (0.0 to 1.0)
	Clutch                  uint8      // Amount of clutch applied (0 to 100)
	Gear                    int8       // Gear selected (1-8, N=0, R=-1)
	EngineRPM               uint16     // Engine RPM
	DRS                     uint8      // DRS - 0 = off, 1 = on
	RevLightsPercent        uint8      // Rev lights indicator (percentage)
	RevLightsBitValue       uint16     // Rev lights (bit 0 = leftmost LED, bit 14 = rightmost LED)
	BrakesTemperature       [4]uint16  // Brakes temperature (celsius)
	TyresSurfaceTemperature [4]uint8   // Tyres surface temperature (celsius)
	TyresInnerTemperature   [4]uint8   // Tyres inner temperature (celsius)
	EngineTemperature       uint16     // Engine temperature (celsius)
	TyresPressure           [4]float32 // Tyres pressure (PSI)
	SurfaceType             [4]uint8   // Driving surface
}

// 120*1349 = 161880bytes/s ~= 159k/s
// CarMotionData represents motion-related data for a single car.
type CarMotionData struct {
	WorldPositionX     float32 // World space X position - metres
	WorldPositionY     float32 // World space Y position
	WorldPositionZ     float32 // World space Z position
	WorldVelocityX     float32 // Velocity in world space X – metres/s
	WorldVelocityY     float32 // Velocity in world space Y
	WorldVelocityZ     float32 // Velocity in world space Z
	WorldForwardDirX   int16   // World space forward X direction (normalised)
	WorldForwardDirY   int16   // World space forward Y direction (normalised)
	WorldForwardDirZ   int16   // World space forward Z direction (normalised)
	WorldRightDirX     int16   // World space right X direction (normalised)
	WorldRightDirY     int16   // World space right Y direction (normalised)
	WorldRightDirZ     int16   // World space right Z direction (normalised)
	GForceLateral      float32 // Lateral G-Force component
	GForceLongitudinal float32 // Longitudinal G-Force component
	GForceVertical     float32 // Vertical G-Force component
	Yaw                float32 // Yaw angle in radians
	Pitch              float32 // Pitch angle in radians
	Roll               float32 // Roll angle in radians
}

// 120*1131=135720bytes/s ~=133k/s
// LapData represents data related to a lap of a car.
type LapData struct {
	LastLapTimeInMS             uint32  // Last lap time in milliseconds
	CurrentLapTimeInMS          uint32  // Current time around the lap in milliseconds
	Sector1TimeInMS             uint16  // Sector 1 time in milliseconds
	Sector1TimeMinutes          uint8   // Sector 1 whole minute part
	Sector2TimeInMS             uint16  // Sector 2 time in milliseconds
	Sector2TimeMinutes          uint8   // Sector 2 whole minute part
	DeltaToCarInFrontInMS       uint16  // Time delta to car in front in milliseconds
	DeltaToRaceLeaderInMS       uint16  // Time delta to race leader in milliseconds
	LapDistance                 float32 // Distance vehicle is around current lap in metres – could be negative if line hasn’t been crossed yet
	TotalDistance               float32 // Total distance travelled in session in metres – could be negative if line hasn’t been crossed yet
	SafetyCarDelta              float32 // Delta in seconds for safety car
	CarPosition                 uint8   // Car race position
	CurrentLapNum               uint8   // Current lap number
	PitStatus                   uint8   // Pit status
	NumPitStops                 uint8   // Number of pit stops taken in this race
	Sector                      uint8   // Sector
	CurrentLapInvalid           uint8   // Current lap invalid
	Penalties                   uint8   // Accumulated time penalties in seconds to be added
	TotalWarnings               uint8   // Accumulated number of warnings issued
	CornerCuttingWarnings       uint8   // Accumulated number of corner cutting warnings issued
	NumUnservedDriveThroughPens uint8   // Num drive through pens left to serve
	NumUnservedStopGoPens       uint8   // Num stop go pens left to serve
	GridPosition                uint8   // Grid position the vehicle started the race in
	DriverStatus                uint8   // Status of driver
	ResultStatus                uint8   // Result status
	PitLaneTimerActive          uint8   // Pit lane timing
	PitLaneTimeInLaneInMS       uint16  // If active, the current time spent in the pit lane in ms
	PitStopTimerInMS            uint16  // Time of the actual pit stop in ms
	PitStopShouldServePen       uint8   // Whether the car should serve a penalty at this stop
}

// 120*1249=148680bytes/s ~=146k/s
type CarStatusData struct{}

// all of these added means in the worst case, we have over 480 packets/s and over half a Mb per second of data to process.
// that doesn't seem to bad, but if we wanted something similar to run on something like an arduino or some DIY pedal/handbrake/etc.,
// we may need to worry more about performance.
