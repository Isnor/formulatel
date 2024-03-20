package formulatel

import (
	"google.golang.org/grpc"

	pb "github.com/isnor/formulatel/internal/genproto"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
)

// var tracer = otel.Tracer("formulatel")

// func initTracer() (*sdktrace.TracerProvider, error) {
// 	ctx := context.Background()

// 	exporter, err := otlptracegrpc.New(ctx)
// 	if err != nil {
// 		return nil, err
// 	}
// 	tp := sdktrace.NewTracerProvider(
// 		sdktrace.WithBatcher(exporter),
// 		// sdktrace.WithResource(initResource()),
// 	)
// 	otel.SetTracerProvider(tp)
// 	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

// 	// TODO: add meter provider
// 	return tp, nil
// }

func FormulaTelServer() *grpc.Server {
	server := grpc.NewServer(grpc.StatsHandler(otelgrpc.NewServerHandler()))
	pb.RegisterCarMotionDataServiceServer(server, &CarMotionService{})
	return server
}
