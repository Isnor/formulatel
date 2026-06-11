begin;

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
    timescaledb.compress,
    timescaledb.compress_segmentby = 'session_id, user_id',
    timescaledb.compress_orderby = 'time DESC',
    timescaledb.chunk_interval = '1 day'
);

end;