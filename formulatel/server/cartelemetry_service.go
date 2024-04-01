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

// TODO: add attributes to the metrics from the context? I'm not sure how we're going to do this. I think
// the answer might be _hammering_ values into the context rather than in some singleton struct like MetricsImpl
// because context is all the asynchronous gauges will be able to use, but the better solution is probably
// to just write our own exporter
func (m *CarTelemetryMetricsImpl) RecordTelemetry(ctx context.Context, telemetry *pb.CarTelemetryData) {
	m.Gauges.Speed.Store(int64(telemetry.Speed))
	m.Gauges.Break.Store(float64(telemetry.Brake))
	m.Gauges.Throttle.Store(float64(telemetry.Throttle))
}

func (m *CarTelemetryMetricsImpl) Record(ctx context.Context, telemetry *pb.CarTelemetryData) {
	m.RecordTelemetry(ctx, telemetry)
}

type CarTelemetryGauges struct {
	Speed    atomic.Int64
	Break    AtomicFloat32 // float32
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

type CarTelemetryService struct {
	FormulaTelMetricsRecorder[pb.CarTelemetryData]
	pb.UnimplementedCarTelemetryDataServiceServer
}

func (c *CarTelemetryService) SendCarTelemetryData(ctx context.Context, data *pb.CarTelemetryData) (*pb.CarTelemetryAck, error) {
	c.FormulaTelMetricsRecorder.Record(ctx, data)
	return &pb.CarTelemetryAck{}, nil
}

func SendData[T any](ctx context.Context, recorder FormulaTelMetricsRecorder[T], data *T) (*pb.CarTelemetryAck, error) {
	recorder.Record(ctx, data)
	return &pb.CarTelemetryAck{}, nil
}
