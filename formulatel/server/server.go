package formulatel

import (
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel/metric"
	"google.golang.org/grpc"
)

func FormulaTelServer(meter metric.Meter) *grpc.Server {

	server := grpc.NewServer(grpc.StatsHandler(otelgrpc.NewServerHandler()))
	// f123.RegisterCarMotionDataServiceServer(server, &CarMotionService{
	// 	CarMotionMetrics: &CarMotionMetricsImpl{},
	// })
	// I was hoping I could loop over the different types of telemetry data - car,
	// motion, session, etc. and register an "anonymous" gRPC handler instead of defining
	// an RPC service in the protobuf for every type of data, because the handler for each one
	// is basically the same thing: convert the data to an open telemetry metric and record it.

	// this raises a question: if ingest is the thing that converts specific telemetry into a
	// general structure, what is RPC even doing?
	// server.RegisterService(...)
	// f123.RegisterCarTelemetryDataServiceServer(server, &CarTelemetryService{})
	return server
}
