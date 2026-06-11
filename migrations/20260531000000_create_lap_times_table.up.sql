begin;

CREATE TABLE IF NOT EXISTS lap_times (
    session_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    title INTEGER NOT NULL,
    lap_num INTEGER NOT NULL,
    lap_time INTERVAL,
    sector1_time INTERVAL,
    sector2_time INTERVAL,
    sector3_time INTERVAL,
    lap_valid BOOL NOT NULL DEFAULT FALSE,
    sector1_valid BOOL NOT NULL DEFAULT FALSE,
    sector2_valid BOOL NOT NULL DEFAULT FALSE,
    sector3_valid BOOL NOT NULL DEFAULT FALSE
);

ALTER TABLE lap_times ADD CONSTRAINT unique_lap UNIQUE (session_id, user_id, lap_num);

end;