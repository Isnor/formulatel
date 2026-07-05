begin;

CREATE TABLE IF NOT EXISTS telemetry.live_lap_data (
    time                    TIMESTAMPTZ NOT NULL,
    session_id              TEXT        NOT NULL,
    user_id                 TEXT        NOT NULL,
    title                   INTEGER     NOT NULL,
    lap_num                 INTEGER     NOT NULL,
    current_lap_time        INTEGER     NOT NULL,
    sector                  INTEGER     NOT NULL,
    sector1_time            INTEGER,
    sector2_time            INTEGER,
    delta_to_car_in_front   INTEGER,
    delta_to_race_leader    INTEGER,
    lap_distance            REAL,
    total_distance          REAL
) WITH (
    timescaledb.hypertable,
    timescaledb.compress,
    timescaledb.compress_segmentby = 'session_id, user_id',
    timescaledb.compress_orderby = 'time DESC',
    timescaledb.chunk_interval = '1 day'
);

end;