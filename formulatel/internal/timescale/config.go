// Package timescale provides the persistence layer for writing telemetry data to TimescaleDB.
package timescale

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
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

// NewConnection creates a new pgx connection pool.
func NewConnection(ctx context.Context, dsn string) (*pgx.Conn, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return nil, err
	}

	return conn, nil
}
