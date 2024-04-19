package formulatel

import (
	"context"
	"log/slog"
	"sync/atomic"

	"github.com/isnor/formulatel/internal/genproto"
	"github.com/segmentio/kafka-go"
	"google.golang.org/protobuf/proto"
)

const (
	VehicleDataTopic = "formulatel-vehicle-data"
)

// this is me trying to write a sort of general protobuf kafka producer. It just wraps the kafka.Writer
// and listens on a channel for messages. The plan is to have a writer per topic/type of telemetry
type KafkaTelemetryProducer struct {
	Writer *kafka.Writer

	Messages chan *genproto.GameTelemetry // receives messages to write on this channel
	Shutdown *atomic.Bool
}

func (t *KafkaTelemetryProducer) Persist(ctx context.Context, data *genproto.GameTelemetry) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		t.Messages <- data
	}
	return nil
}

// ProduceMessages reads from t.Messages and writes to a Kafka topic.
// This is a blocking function that runs until the producer is shutdown or its
// Messages channel is closed
func (t *KafkaTelemetryProducer) ProduceMessages(ctx context.Context) {
	if t.Shutdown.Load() {
		return
	}
	for message := range t.Messages {
		protoBytes, err := proto.Marshal(message)
		if err != nil {
			// TODO: handle better
			slog.ErrorContext(ctx, "failed serializing a message")
			continue
		}
		t.Writer.WriteMessages(ctx, kafka.Message{Value: protoBytes})
		if t.Shutdown.Load() {
			break
		}
	}
	slog.Info("kafka finished producing messages")
}

// TODO: look into consumer groups
type KafkaTelemetryConsumer struct {
	Reader *kafka.Reader

	Shutdown *atomic.Bool
}

func (c *KafkaTelemetryConsumer) ReadTelemetry(ctx context.Context) (*genproto.GameTelemetry, error) {
	msg, err := c.Reader.ReadMessage(ctx)
	if err != nil {
		return nil, err
	}
	var res genproto.GameTelemetry
	if err := proto.Unmarshal(msg.Value, &res); err != nil {
		return nil, err
	}
	return &res, nil
}
