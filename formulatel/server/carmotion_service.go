package formulatel

import (
	"context"
	"fmt"

	pb "github.com/isnor/formulatel/internal/genproto"
)

type CarMotionService struct {
	pb.UnimplementedCarMotionDataServiceServer
}

func (c *CarMotionService) SendCarMotionData(ctx context.Context, data *pb.CarMotionData) (*pb.CarMotionAck, error) {
	// TODO: add metrics and metrics exporter
	fmt.Printf("%+v\n", data)
	return &pb.CarMotionAck{}, nil
}

// func (c *CarMotionService) StreamCarMotionData(pb.CarMotionDataService_StreamCarMotionDataServer) error {
// 	return nil
// }
