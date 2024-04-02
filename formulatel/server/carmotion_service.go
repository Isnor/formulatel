package formulatel

// One way we could do metrics is by having a struct per service that has methods that correspond to the data
// we want to visualize. Initially I wanted to make things more generic
// type CarMotionMetrics interface {
// 	RecordMotion(context.Context, *pb.CarMotionData)
// }

// type CarMotionMetricsImpl struct {
// 	MotionPacketsCounter metric.Int64Counter
// }

// // do something with the motion data
// func (m *CarMotionMetricsImpl) RecordMotion(ctx context.Context, motion *pb.CarMotionData) {
// 	m.MotionPacketsCounter.Add(ctx, 1)
// }

// type CarMotionService struct {
// 	CarMotionMetrics
// 	pb.UnimplementedCarMotionDataServiceServer
// }

// func (c *CarMotionService) SendCarMotionData(ctx context.Context, data *pb.CarMotionData) (*pb.CarMotionAck, error) {

// 	c.RecordMotion(ctx, data)
// 	fmt.Printf("%+v\n", data)
// 	return &pb.CarMotionAck{}, nil
// }
