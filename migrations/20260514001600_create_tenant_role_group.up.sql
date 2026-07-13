-- the telemetry_readers role represents a "group" of users who can read telemetry.
-- all of the tenant roles are part of this group
CREATE ROLE telemetry_readers WITH NOLOGIN;
GRANT USAGE ON SCHEMA telemetry TO telemetry_readers;
GRANT SELECT ON ALL TABLES IN SCHEMA telemetry TO telemetry_readers;
ALTER DEFAULT PRIVILEGES IN SCHEMA telemetry GRANT SELECT ON TABLES TO telemetry_readers;