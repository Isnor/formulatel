package formulatel

// This file contains the highest level of abstractions my small mind could cobble together. They
// are extremely general and simple with the hope of providing some sense of consistency while
// connecting arbitrary telemetry sources together. There may well be no point to this at all.
//
// FormulaTelPersist is a sort of concrete implementation of "read from X and write it to Y",
// whereas FormulaTelIngest is more of a container to indicate what an ingestion service should do

import (
	"context"
	"log/slog"
	"sync/atomic"

	"github.com/isnor/formulatel/internal/genproto"
)

// TelemetryPersistor describes something that can persist Formulatel telemetry to an external store
// For example, an implementation could persist to a kafka topic or an opensearch cluster
type TelemetryPersistor interface {
	Persist(context.Context, *genproto.GameTelemetry) error
}

// TelemetryReader describes something that can read telemetry
type TelemetryReader interface {
	ReadTelemetry(context.Context) (*genproto.GameTelemetry, error)
}

// FormulaTelPersist combines a TelemetryPersistor and Reader to create a pipeline that reads telemetry
// and persists it to an external source. This is an attempt to allow "arbitrary" telemetry
// sources and destinations.
type FormulaTelPersist struct {
	TelemetryReader
	TelemetryPersistor

	Shutdown *atomic.Bool
}

// Run reads telemetry and persists it depending on the reader and persistor
func (f *FormulaTelPersist) Run(ctx context.Context) {
	defer slog.DebugContext(ctx, "finished persisting")
	for !f.Shutdown.Load() {
		slog.DebugContext(ctx, "reading")
		t, err := f.ReadTelemetry(ctx)
		if err != nil {
			// TODO: probably should do something about this
			slog.ErrorContext(ctx, "failed reading telemetry", "error", err.Error())
			continue
		}
		slog.DebugContext(ctx, "read telemetry")
		if err = f.Persist(ctx, t); err != nil {
			// TODO: probably should do something about this
			slog.ErrorContext(ctx, "failed persisting telemetry", "error", err.Error())
			continue
		}
		slog.DebugContext(ctx, "persisted telemetry")
	}
}