begin;

CREATE TABLE IF NOT EXISTS telemetry.motion_data (
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
    timescaledb.compress,
    timescaledb.compress_segmentby = 'session_id, user_id',
    timescaledb.compress_orderby = 'time DESC',
    timescaledb.chunk_interval = '1 day'
);

end;