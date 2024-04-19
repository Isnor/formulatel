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

	Messages  chan *genproto.GameTelemetry // receives messages to write on this channel
	Shutdown  *atomic.Bool
	BatchSize int
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

// ProduceMessages reads from t.Messages and writes to a Kafka topic
func (t *KafkaTelemetryProducer) ProduceMessages(ctx context.Context) {
	if t.Shutdown.Load() {
		return
	}
	currentBatch := make([]kafka.Message, 0, t.BatchSize)
	defer func() {
		if len(currentBatch) != 0 {
			t.Writer.WriteMessages(ctx, currentBatch...)
			clear(currentBatch)
		}
	}()
	for message := range t.Messages {
		protoBytes, err := proto.Marshal(message)
		if err != nil {
			// TODO: handle better
			slog.ErrorContext(ctx, "failed serializing a message")
			continue
		}
		currentBatch = append(currentBatch, kafka.Message{Value: protoBytes})
		if len(currentBatch) >= t.BatchSize {
			t.Writer.WriteMessages(ctx, currentBatch...)
			clear(currentBatch)
		}
		if t.Shutdown.Load() {
			break
		}
	}
	slog.Info("kafka finished producing messages")
}

type KafkaTelemetryConsumer struct {
	Reader *kafka.Reader

	Shutdown *atomic.Bool
	// BatchSize int
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
