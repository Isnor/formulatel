CREATE TABLE IF NOT EXISTS telemetry.extended_wheel_data (
    time                  TIMESTAMPTZ NOT NULL,
    session_id            TEXT        NOT NULL,
    user_id               TEXT        NOT NULL,
    title                 INTEGER     NOT NULL,

    -- Back Left Wheel
    bl_wheel_speed        REAL NOT NULL,
    bl_vertical_force     REAL NOT NULL,
    bl_slip_angle         REAL NOT NULL,
    bl_slip_ratio         REAL NOT NULL,
    bl_lateral_force      REAL NOT NULL,
    bl_longitudinal_force REAL NOT NULL,
    bl_suspension_position REAL,
    bl_suspension_velocity REAL,

    -- Back Right Wheel
    br_wheel_speed        REAL NOT NULL,
    br_vertical_force     REAL NOT NULL,
    br_slip_angle         REAL NOT NULL,
    br_slip_ratio         REAL NOT NULL,
    br_lateral_force      REAL NOT NULL,
    br_longitudinal_force REAL NOT NULL,
    br_suspension_position REAL,
    br_suspension_velocity REAL,

    -- Front Left Wheel
    fl_wheel_speed        REAL NOT NULL,
    fl_vertical_force     REAL NOT NULL,
    fl_slip_angle         REAL NOT NULL,
    fl_slip_ratio         REAL NOT NULL,
    fl_lateral_force      REAL NOT NULL,
    fl_longitudinal_force REAL NOT NULL,
    fl_suspension_position REAL,
    fl_suspension_velocity REAL,

    -- Front Right Wheel
    fr_wheel_speed        REAL NOT NULL,
    fr_vertical_force     REAL NOT NULL,
    fr_slip_angle         REAL NOT NULL,
    fr_slip_ratio         REAL NOT NULL,
    fr_lateral_force      REAL NOT NULL,
    fr_longitudinal_force REAL NOT NULL,
    fr_suspension_position REAL,
    fr_suspension_velocity REAL

) WITH (
    timescaledb.hypertable,
    timescaledb.compress,
    timescaledb.compress_segmentby = 'session_id, user_id',
    timescaledb.compress_orderby = 'time DESC',
    timescaledb.chunk_interval = '1 day'
);
