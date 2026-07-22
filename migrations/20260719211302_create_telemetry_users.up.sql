BEGIN;

CREATE TABLE IF NOT EXISTS telemetry.users (
  id UUID NOT NULL PRIMARY KEY,
  tenant_id INT,
  username VARCHAR(100) NOT NULL
);

ALTER TABLE telemetry.users ENABLE ROW LEVEL SECURITY;

CREATE POLICY tenant_telemetry_readers ON telemetry.users USING (tenant_id = substring(CURRENT_USER from 'tenant_([0-9]+)')::int);

GRANT SELECT ON telemetry.users TO telemetry_readers;

COMMIT;