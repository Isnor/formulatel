package formulatel

import (
	"google.golang.org/grpc"

	pb "github.com/isnor/formulatel/internal/genproto"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
)

func FormulaTelServer() *grpc.Server {
	server := grpc.NewServer(grpc.StatsHandler(otelgrpc.NewServerHandler()))
	pb.RegisterCarMotionDataServiceServer(server, &CarMotionService{})
	return server
}
