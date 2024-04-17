package formulatel

import (
	"context"
	"log/slog"
	"sync/atomic"

	"github.com/segmentio/kafka-go"
	"google.golang.org/protobuf/proto"
)

// this is me trying to write a sort of general protobuf kafka producer. It just wraps the kafka.Writer
// and listens on a channel for messages
type AsyncTelemetryProducer[T proto.Message] struct {
	Writer *kafka.Writer

	Messages  chan T // receives messages to write on this channel
	Shutdown  *atomic.Bool
	BatchSize int
}

func (t *AsyncTelemetryProducer[_]) ProduceMessages(ctx context.Context) {
	currentBatch := make([]kafka.Message, 0, t.BatchSize)
	defer func() {
		if len(currentBatch) != 0 {
			t.Writer.WriteMessages(ctx, currentBatch...)
			clear(currentBatch)
		}
	}()
	for !t.Shutdown.Load() {
		message, closed := <-t.Messages
		if closed {
			break
		}
		println("bababooy")
		x, err := proto.Marshal(message)
		if err != nil {
			slog.ErrorContext(ctx, "failed serializing a message")
			continue
		}
		currentBatch = append(currentBatch, kafka.Message{Value: x})
		if len(currentBatch) >= t.BatchSize {
			t.Writer.WriteMessages(ctx, currentBatch...)
			clear(currentBatch)
		}
	}
	slog.Info("kafka finished producing messages")
}
