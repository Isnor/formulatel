package formulatel

import (
	"context"
	"sync/atomic"

	pb "github.com/isnor/formulatel/internal/genproto"
	"go.opentelemetry.io/otel/metric"
)

// FormulaTelMetricsRecorder describes something that records metrics pertaining to T. Implementations should be
// thread-safe
type FormulaTelMetricsRecorder[T any] interface {
	Record(context.Context, *T)
}

type CarTelemetryMetricsImpl struct {
	Gauges *CarTelemetryGauges
}

type CarTelemetryGauges struct {
	Speed    atomic.Int64
	Break    AtomicFloat32
	Throttle AtomicFloat32
}

type AtomicFloat32 struct {
	atomic.Value
}

func (a *AtomicFloat32) Load() float32 {
	return a.Value.Load().(float32)
}

// NewGauges creates a new instance of CarTelemetryGauges, which is the in-memory representation of a car's telemetry data.
// As telemetry comes into the service, CarTelemetryGauges keeps track of the latest value for each sensor of the car.
// This function registers those gauges with similarly named opentelemetry Gauges
func NewGauges(meter metric.Meter) *CarTelemetryGauges {
	gauges := &CarTelemetryGauges{}
	meter.Int64ObservableGauge("formulatelrpc.speed.gauge", metric.WithInt64Callback(func(ctx context.Context, o metric.Int64Observer) error {
		o.Observe(gauges.Speed.Load())
		return nil
	}))
	meter.Float64ObservableGauge("formulatelrpc.break.gauge", metric.WithFloat64Callback(func(ctx context.Context, fo metric.Float64Observer) error {
		fo.Observe(float64(gauges.Break.Load()))
		return nil
	}))
	meter.Float64ObservableGauge("formulatelrpc.throttle.gauge", metric.WithFloat64Callback(func(ctx context.Context, fo metric.Float64Observer) error {
		fo.Observe(float64(gauges.Throttle.Load()))
		return nil
	}))
	return gauges
}

type TelemetryData[T any] struct {
	Data     *T
	Recorder FormulaTelMetricsRecorder[T]
	pb.UnimplementedVehicleTelemetryServiceServer
}

func (t *TelemetryData[T]) SendGameTelemetry(s pb.VehicleTelemetryService_SendGameTelemetryServer) error {
	// this was supposed to be a more generic way of recording the telemetry data, but it looks like I got confused
	// I was hoping to declare this generally and create instances of TelemetryData[pb.VehicleData], TelemetryData[pb.LapData],
	// etc. with the appropriate recorder, but it looks like I can't do that
	// tel, err := s.Recv()
	// if err != nil {
	// 	return err
	// }
	// t.Recorder.Record(s.Context(), tel)
	return nil
}

func SendData[T any](ctx context.Context, recorder FormulaTelMetricsRecorder[T], data *T) (*pb.TelemetryAck, error) {
	recorder.Record(ctx, data)
	return &pb.TelemetryAck{}, nil
}
