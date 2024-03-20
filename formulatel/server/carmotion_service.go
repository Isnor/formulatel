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
	// TODO: span := trace.SpanFromContext(ctx), use span(?) it may not be that useful for this application
	// the client is going to be putting the data/attributes on the span, so at this point this only thing we can do
	// (maybe?) is propogate that into the datastore calls. I think that'll largely be done with the context though,
	// using the instrumentation libraries
	// tracer.Start(ctx, "CarMotion", trace.WithAttributes())
	fmt.Printf("%+v\n", data)
	return &pb.CarMotionAck{}, nil
}

// func (c *CarMotionService) StreamCarMotionData(pb.CarMotionDataService_StreamCarMotionDataServer) error {
// 	return nil
// }
