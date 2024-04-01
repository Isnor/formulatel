package formulatel

import (
	pb "github.com/isnor/formulatel/internal/genproto"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel/metric"
	"google.golang.org/grpc"
)

func FormulaTelServer(meter metric.Meter) *grpc.Server {

	server := grpc.NewServer(grpc.StatsHandler(otelgrpc.NewServerHandler()))
	pb.RegisterCarMotionDataServiceServer(server, &CarMotionService{
		CarMotionMetrics: &CarMotionMetricsImpl{
		},
	})

	pb.RegisterCarTelemetryDataServiceServer(server, &CarTelemetryService{})
	return server
}
