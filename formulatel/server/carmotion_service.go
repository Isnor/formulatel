package formulatel

import (
	"context"
	"fmt"

	pb "github.com/isnor/formulatel/internal/genproto"
	"go.opentelemetry.io/otel/metric"
)

type CarMotionService struct {
	MotionPacketsCounter metric.Int64Counter
	pb.UnimplementedCarMotionDataServiceServer
}

func (c *CarMotionService) SendCarMotionData(ctx context.Context, data *pb.CarMotionData) (*pb.CarMotionAck, error) {
	
	c.MotionPacketsCounter.Add(ctx, 1)

	fmt.Printf("%+v\n", data)
	return &pb.CarMotionAck{}, nil
}

// func (c *CarMotionService) StreamCarMotionData(pb.CarMotionDataService_StreamCarMotionDataServer) error {
// 	return nil
// }
