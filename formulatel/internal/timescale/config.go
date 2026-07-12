// Package timescale provides the persistence layer for writing telemetry data to TimescaleDB.
package timescale

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrInvalidConfig = errors.New("invalid config")

// TODO: why is this config here?
// Config holds configuration for the TimescaleDB persistence service.
// Fields are tagged to bind environment variables automatically.
type Config struct {
	TimescaleDSN  string        `envconfig:"TIMESCALE_DSN"`
	DBUser        string        `envconfig:"DB_USER"`
	DBPassword    string        `envconfig:"DB_PASSWORD"`
	DBHost        string        `envconfig:"DB_HOST"`
	DBPort        uint16        `envconfig:"DB_PORT"`
	DBName        string        `envconfig:"DB_NAME"`
	MQTTBroker    string        `envconfig:"MQTT_BROKER" required:"true"`
	MQTTClientID  string        `envconfig:"MQTT_CLIENT_ID"`
	BatchSize     int           `split_words:"true" default:"500"`
	FlushInterval time.Duration `split_words:"true" default:"10s"`
}

// Validate returns nil if the config was valid or an error indicating why the config was considered invalid.
func (c Config) Validate() error {
	if c.TimescaleDSN == "" && (c.DBUser == "" || c.DBPassword == "" || c.DBHost == "" || c.DBName == "" || c.DBPort == 0) {
		return fmt.Errorf("%w: either TimescaleDSN or all of {DB_USER, DB_PASSWORD, DB_HOST, DB_PORT, DB_NAME} must be set", ErrInvalidConfig)
	}
	return nil
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
