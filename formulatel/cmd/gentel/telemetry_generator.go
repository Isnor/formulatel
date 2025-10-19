package main

import (
	"context"
	"log/slog"
	"time"

	pb "github.com/isnor/formulatel/internal/genproto"
)

// coefficients to apply to the rising RPM in different gears
var powerMapping map[int32]float32 = map[int32]float32{
	0:  0.0,
	1:  150,
	2:  100,
	3:  70,
	4:  60,
	5:  50,
	6:  40,
	7:  30,
	8:  20,
	9:  10,
	10: 2,
}

const kphToNmMS = 277778

type TelemetryGenerator struct {
	MaxGear   uint32
	MaxRPM    uint32
	RPMRate   uint32
	MaxSpeed  float32
	SpeedRate float32 // how much the speed goes up per tick, in nm/ms

	// TODO: might be better to let the programmer access `ticker` so that they
	// could reset it if they wanted?
	ticksPerSecond int // how many ticks per second
	ticker         *time.Ticker
}

func NewTelemetryGenerator(frequency int) *TelemetryGenerator {
	if frequency <= 0 {
		frequency = 1
	}
	t := time.NewTicker(time.Second / time.Duration(frequency))
	return &TelemetryGenerator{
		ticker:         t,
		ticksPerSecond: frequency,
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

// 1k/h -> 277778nm/ms . If speed is really going to be an integer, we need to use a unit
// other than kph in order to increase it per tick (because we can't add .01 kph, but can add 10000 nm/ms)
func (g *TelemetryGenerator) NextTelemetry(last time.Time, previous *pb.VehicleData) *pb.VehicleData {
	// I guess I shouldn't be copying the lock by value here, but I really don't care about it,
	// I just want to use the data. I wonder what I should be doing
	var x = *previous

	var timeSinceLast = time.Since(last)
	// this function should run every "tick", but if there was a delay for some reason, we should try to account
	// for it by determining how many ticks have elapsed from now and last
	// how much time since the last tick have elapsed / how often each tick should occur
	var ticksSinceLast = timeSinceLast.Nanoseconds() / (time.Second / time.Duration(g.ticksPerSecond)).Nanoseconds()
	slog.Debug("creating new telemetry",
		"time_since_last", timeSinceLast,
		"ticks_since_last", ticksSinceLast, // should always be one unless there was a delay
	)

	if previous.Speed >= uint32(g.MaxSpeed) && previous.Rpm >= g.MaxRPM && previous.Gear >= int32(g.MaxGear) {
		// we're at the limit, just increase the temperature
		x.EngineTemperature += 10
		return &x
	}
	x.EngineTemperature += 2

	speedIncrease := g.SpeedRate * float32(ticksSinceLast)
	if previous.Speed < uint32(g.MaxSpeed) {
		// bit annoying that we can only increase the speed by steps of 1, maybe we should change this
		// instead, I changed its meaning from "k / h" to "nm / ms"
		slog.Debug("increased speed",
			"increase", speedIncrease/kphToNmMS,
			"speed_rate", g.SpeedRate/kphToNmMS,
			"tsl", timeSinceLast,
		)
		x.Speed += uint32(speedIncrease / 2)
	}

	if previous.Rpm < g.MaxRPM {
		x.Rpm += uint32(powerMapping[x.Gear] * speedIncrease / kphToNmMS)
	} else if x.Gear != int32(g.MaxGear) {
		x.Gear += 1
		x.Rpm -= 7000
		x.Speed -= 10
	}

	return &x
}
