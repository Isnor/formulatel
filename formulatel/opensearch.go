package formulatel

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/isnor/formulatel/internal/genproto"
	opensearch "github.com/opensearch-project/opensearch-go/v2"
	"google.golang.org/protobuf/encoding/protojson"
)

type OpenSearchTelemetryPersistor struct {
	OpenSearch *opensearch.Client
	Index      string // the index to put documents in; the idea is to have a persistor per index
}

func (p *OpenSearchTelemetryPersistor) Persist(ctx context.Context, data *genproto.GameTelemetry) error {
	// TODO: this doesn't do any batching and it would probably be more efficient if it did
	protoJSON, err := protojson.Marshal(data)
	if err != nil {
		return err
	}
	// docID - uuid for the document, can be the Kafka key
	// TODO: we don't have the kafka key or any context of kafka here.
	slog.DebugContext(ctx, "writing data to opensearch")
	_, err = p.OpenSearch.Create(p.Index, fmt.Sprintf("%d", time.Now().UnixNano()), bytes.NewBuffer(protoJSON))
	return err
}
