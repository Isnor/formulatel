package main

import (
	"context"
	"log/slog"
	"time"

	pb "github.com/isnor/formulatel/internal/genproto"
)

type TelemetryGenerator struct {
	MaxGear   uint32
	MaxRPM    uint32
	RPMRate   uint32
	MaxSpeed  float32
	SpeedRate float32 // how much the speed goes up per tick

	frequency int // how many ticks per second
	ticker    *time.Ticker
}

func NewTelemetryGenerator(frequency int) *TelemetryGenerator {
	if frequency <= 0 {
		frequency = 1
	}
	t := time.NewTicker(time.Second / time.Duration(frequency))
	return &TelemetryGenerator{
		ticker: t,
		frequency: frequency,
	}
}

func (g *TelemetryGenerator) GenerateLoop(ctx context.Context, handleFunc func(t *pb.VehicleData)) error {
	// this is the initial state
	previousTel := &pb.VehicleData{
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
	}
	previousTickTime := <-g.ticker.C
	for {
		select {
		case <-ctx.Done():
			return nil
		case now := <-g.ticker.C:
			// TODO: can we make this more confusing if we use recursion?
			nextTel := g.NextTelemetry(previousTickTime, previousTel)
			handleFunc(nextTel)
			previousTickTime = now
			previousTel = nextTel
		}
	}
}

func (g *TelemetryGenerator) NextTelemetry(last time.Time, previous *pb.VehicleData) *pb.VehicleData {
	var x = *previous
	var timeSinceLast = time.Since(last)
	slog.Debug("creating new telemetry", "time-since-last", timeSinceLast)

	if previous.Speed >= uint32(g.MaxSpeed) && previous.Rpm >= g.MaxRPM && previous.Gear >= int32(g.MaxGear) {
		// we're at the limit, just increase the temperature
		x.EngineTemperature += 10
		return &x
	}
	// bit annoying that we can only increase the speed by steps of 1, maybe we should change this
	speedIncrease := uint32(g.SpeedRate * float32(timeSinceLast.Milliseconds()) / float32(g.frequency))
	slog.Debug("increased speed", "increase", speedIncrease, "speed_rate", g.SpeedRate, "tsl", timeSinceLast.Seconds())
	x.Speed += speedIncrease
	x.EngineTemperature += 2

	if previous.Rpm < g.MaxRPM {
		x.Rpm += speedIncrease * 100
	} else if x.Gear != int32(g.MaxGear) {
		x.Gear += 1
		x.Rpm -= 9000
		x.Speed -= 10
	}

	return &x
}
