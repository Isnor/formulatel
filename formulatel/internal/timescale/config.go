// Package timescale provides the persistence layer for writing telemetry data to TimescaleDB.
package timescale

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Config holds configuration for the TimescaleDB persistence service.
// Fields are tagged to bind environment variables automatically.
type Config struct {
	TimescaleDSN  string        `envconfig:"TIMESCALE_DSN" required:"true"`
	MQTTBroker    string        `envconfig:"MQTT_BROKER" required:"true"`
	MQTTPrefix    string        `envconfig:"MQTT_PREFIX" default:"formulatel"`
	BatchSize     int           `split_words:"true" default:"500"`
	FlushInterval time.Duration `split_words:"true" default:"10s"`
}

// NewConnectionPool creates a new pgx connection pool.
func NewConnectionPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	// TODO: connection timeout should be configurable; honestly this whole function belongs in `main`
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	conn, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}

	return conn, nil
}
