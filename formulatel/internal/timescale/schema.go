package timescale

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// VehicleDataSchema represents the SQL DDL for the vehicle_data hypertable.
const VehicleDataSchema = `
CREATE TABLE IF NOT EXISTS vehicle_data (
    time                 TIMESTAMPTZ NOT NULL,
    session_id           TEXT        NOT NULL,
    user_id              TEXT        NOT NULL,
    title                INTEGER     NOT NULL,
    speed                INTEGER     NOT NULL,
    rpm                  INTEGER     NOT NULL,
    throttle             REAL        NOT NULL,
    brake                REAL        NOT NULL,
    steering             REAL        NOT NULL,
    gear                 INTEGER     NOT NULL,
    engine_temperature   INTEGER     NOT NULL,
    -- Tires (nullable, flattened)
    fl_brake_temp        INTEGER, fl_inner_temp INTEGER,
    fl_surface_temp      INTEGER, fl_pressure   INTEGER,
    fr_brake_temp        INTEGER, fr_inner_temp INTEGER,
    fr_surface_temp      INTEGER, fr_pressure   INTEGER,
    bl_brake_temp        INTEGER, bl_inner_temp INTEGER,
    bl_surface_temp      INTEGER, bl_pressure   INTEGER,
    br_brake_temp        INTEGER, br_inner_temp INTEGER,
    br_surface_temp      INTEGER, br_pressure   INTEGER
) WITH (
    timescaledb.hypertable,
    timescaledb.chunk_interval = '1 day'
);
`

// MotionDataSchema represents the SQL DDL for the motion_data hypertable.
const MotionDataSchema = `
CREATE TABLE IF NOT EXISTS motion_data (
    time                  TIMESTAMPTZ NOT NULL,
    session_id            TEXT        NOT NULL,
    user_id               TEXT        NOT NULL,
    title                 INTEGER     NOT NULL,
    position_x            REAL NOT NULL, position_y REAL NOT NULL, position_z REAL NOT NULL,
    velocity_x            REAL NOT NULL, velocity_y REAL NOT NULL, velocity_z REAL NOT NULL,
    gforce_lateral        REAL NOT NULL, gforce_longitudinal REAL NOT NULL, gforce_vertical REAL NOT NULL,
    yaw                   REAL NOT NULL, pitch REAL NOT NULL, roll REAL NOT NULL
) WITH (
    timescaledb.hypertable,
    timescaledb.chunk_interval = '1 day'
);
`

// EnsureSchema ensures the TimescaleDB hypertables exist.
func EnsureSchema(ctx context.Context, conn *pgx.Conn) error {
	_, err := conn.Exec(ctx, VehicleDataSchema)
	if err != nil {
		return fmt.Errorf("failed to create vehicle_data hypertable: %w", err)
	}

	_, err = conn.Exec(ctx, MotionDataSchema)
	if err != nil {
		return fmt.Errorf("failed to create motion_data hypertable: %w", err)
	}

	return nil
}
