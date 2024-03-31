package formulatel

import (
	pb "github.com/isnor/formulatel/internal/genproto"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel/metric"
	"google.golang.org/grpc"
)

func FormulaTelServer(meter metric.Meter) *grpc.Server {

	// meter := otel.Meter("formulatel")
	apiCounter, err := meter.Int64Counter(
		"formulatelrpc.motion.packets",
		metric.WithDescription("Number of motion type packets received by formulatel"),
		metric.WithUnit("{packets}"),
	)
	if err != nil {
		panic(err) // TODO: remove this
	}
	server := grpc.NewServer(grpc.StatsHandler(otelgrpc.NewServerHandler()))
	pb.RegisterCarMotionDataServiceServer(server, &CarMotionService{
		CarMotionMetrics: &CarMotionMetricsImpl{
			MotionPacketsCounter: apiCounter,
		},
	})
	pb.RegisterCarTelemetryDataServiceServer(server, &CarTelemetryService{
		CarTelemetryMetrics: &CarTelemetryMetricsImpl{
			Gauges: NewGauges(meter),
		},
	})
	return server
}
