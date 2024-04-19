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
	for !f.Shutdown.Load() {
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

// FormulaTelIngest is very similar to FormulaTelPersist, but Ingest is more of a cosmetic organizational tool
// to put my mind at ease. For now, I couldn't figure out a better way of abstracting the way different titles
// could send us telemetry, so this sort of works.
// I had second thoughts about this and for now, am no longer interested in this abstraction
// type FormulaTelIngest struct {
// 	// read from the source of telemetry - e.g. read packets from over UDP
// 	GameSpecificTelemetryReader TelemetryReader
// 	// write the telemetry from the TelemetryReader - e.g. write to a kafka topic
// 	TelemetryPersistor
// }
