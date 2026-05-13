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
	TimescaleDSN  string        `env:"TIMESCALE_DSN" default:"postgres://postgres:postgres@localhost:5432/formulatel?sslmode=disable"`
	MQTTBroker    string        `env:"MQTT_BROKER" default:"tcp://localhost:1883"`
	MQTTPrefix    string        `env:"MQTT_PREFIX" default:"formulatel"`
	BatchSize     int           `env:"BATCH_SIZE" default:"500"`
	FlushInterval time.Duration `env:"FLUSH_INTERVAL" default:"10s"`
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
