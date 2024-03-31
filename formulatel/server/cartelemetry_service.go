package formulatel

import (
	"context"

	pb "github.com/isnor/formulatel/internal/genproto"
	"go.opentelemetry.io/otel/metric"
)

type CarTelemetryMetrics interface {
	RecordTelemetry(context.Context, *pb.CarTelemetryData)
}

type CarTelemetryMetricsImpl struct {
	Gauges *CarTelemetryGauges
}

// TODO: add attributes to the metrics from the context? I'm not sure how we're going to do this
func (m *CarTelemetryMetricsImpl) RecordTelemetry(ctx context.Context, telemetry *pb.CarTelemetryData) {
	m.Gauges.Speed = int64(telemetry.Speed)
	m.Gauges.Break = float64(telemetry.Brake)
	m.Gauges.Throttle = float64(telemetry.Throttle)
}

type CarTelemetryGauges struct {
	Speed    int64
	Break    float64
	Throttle float64
}

func NewGauges(meter metric.Meter) *CarTelemetryGauges {
	gauges := &CarTelemetryGauges{}
	meter.Int64ObservableGauge("formulatelrpc.speed.gauge", metric.WithInt64Callback(func(ctx context.Context, o metric.Int64Observer) error {
		o.Observe(gauges.Speed)
		return nil
	}))
	meter.Float64ObservableGauge("formulatelrpc.break.gauge", metric.WithFloat64Callback(func(ctx context.Context, fo metric.Float64Observer) error {
		fo.Observe(gauges.Break)
		return nil
	}))
	meter.Float64ObservableGauge("formulatelrpc.throttle.gauge", metric.WithFloat64Callback(func(ctx context.Context, fo metric.Float64Observer) error {
		fo.Observe(gauges.Throttle)
		return nil
	}))
	return gauges
}

type CarTelemetryService struct {
	CarTelemetryMetrics
	pb.UnimplementedCarTelemetryDataServiceServer
}

func (c *CarTelemetryService) SendCarTelemetryData(ctx context.Context, data *pb.CarTelemetryData) (*pb.CarTelemetryAck, error) {
	c.CarTelemetryMetrics.RecordTelemetry(ctx, data)
	return &pb.CarTelemetryAck{}, nil
}
