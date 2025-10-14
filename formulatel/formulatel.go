package formulatel

// TODO: I'm not sure what purpose this serves

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/isnor/formulatel/internal/genproto"
)

// TelemetryPersistor describes something that can persist Formulatel telemetry to an external store
type TelemetryPersistor interface {
	// Persist should write a single GameTelemetry and return an error if the write was unsuccesful
	// for any reason.
	Persist(context.Context, *genproto.GameTelemetry) error
}

// TelemetryReader describes something that can read telemetry.
type TelemetryReader interface {
	// ReadTelemetry should read a single "piece" of telemetry, i.e. whatever the smallest unit of data
	// required to create a single GameTelemetry object.
	ReadTelemetry(context.Context) (*genproto.GameTelemetry, error)
}

// FormulaTelPersist combines a TelemetryPersistor and Reader to create a pipeline that reads telemetry
// and persists it to an external source. This is an attempt to allow "arbitrary" telemetry
// sources and destinations.
type FormulaTelPersist struct {
	TelemetryReader
	TelemetryPersistor
}

// Run blocks, reading telemetry and persisting it depending on the reader and persistor. It reads a single
// GameTelemetry and persists it sequentially, so it's _possible_ there is room for improvement here.
func (f *FormulaTelPersist) Run(ctx context.Context) error {
	if f.TelemetryReader == nil || f.TelemetryPersistor == nil {
		return fmt.Errorf("formulatel persist not initialized")
	}
	defer slog.DebugContext(ctx, "finished persisting")
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
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
}
