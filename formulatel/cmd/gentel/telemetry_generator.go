package main

import (
	"context"
	"time"

	pb "github.com/isnor/formulatel/internal/genproto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type TelemetryGenerator struct {
	MaxGear   int
	MaxRPM    int
	RPMRate   int
	MaxSpeed  float32
	SpeedRate int // how much the speed goes up per tick
	ticker    *time.Ticker
}

func NewTelemetryGenerator(frequency int) *TelemetryGenerator {
	if frequency <= 0 {
		frequency = 1
	}
	t := time.NewTicker(time.Duration(frequency) / time.Second)
	return &TelemetryGenerator{
		ticker: t,
	}
}

func (g *TelemetryGenerator) GenerateLoop(ctx context.Context, handleFunc func(t *pb.GameTelemetry)) error {
	// this is the initial state
	x := &pb.GameTelemetry{
		Title:     pb.GameTitle_GAME_TITLE_UNKNOWN,
		SessionId: "0000000000",
		UserId:    "9999999999",
		Timestamp: timestamppb.Now(),
		Data: &pb.GameTelemetry_VehicleData{
			VehicleData: &pb.VehicleData{
				// let's say we're going to simulate a car speeding up. as time goes up, the speed goes up along
				// with RPM. eventually the RPM hits a limit and the gear increases. eventually the gears, speed,
				// and RPM reach a limit
				Speed:             0,
				Rpm:               1000,
				Throttle:          0,
				Break:             0,
				Steering:          0,
				Gear:              1,
				EngineTemperature: 100,
				Tires: &pb.VehicleData_Tires{
					FrontLeft:  &pb.TireData{},
					FrontRight: &pb.TireData{},
					BackLeft:   &pb.TireData{},
					BackRight:  &pb.TireData{},
				},
			},
		},
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-g.ticker.C:
			handleFunc(x)
		}
	}
}

func (g *TelemetryGenerator) NextTelemetry(start time.Time, previous *pb.VehicleData) *pb.VehicleData {
	var x = *previous
	if previous.Gear == int32(g.MaxGear) {
		if previous.Rpm >= uint32(g.MaxRPM) {
			if previous.Speed >= uint32(g.MaxSpeed) {
				x.EngineTemperature += 10
				return &x
			} else {
				x.Speed += uint32(g.SpeedRate)
			}
		}
	}
	return nil
}
