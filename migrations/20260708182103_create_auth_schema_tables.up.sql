BEGIN;

CREATE SCHEMA IF NOT EXISTS auth;

-- create org table
-- group users by their Grafana org ID
CREATE TABLE IF NOT EXISTS auth.tenants (
  grafana_org_id INT PRIMARY KEY,
  organization_name VARCHAR(255) NOT NULL,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- create users table
-- uniquely identify our users; must be generic enough to support different domains
CREATE TABLE IF NOT EXISTS auth.accounts (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  grafana_org_id INT NOT NULL REFERENCES auth.tenants(grafana_org_id) ON DELETE CASCADE,
  username VARCHAR(100) NOT NULL UNIQUE,
  password_hash VARCHAR(255) NOT NULL, -- password hash bcrypt
  -- human users login to the dashboard and read telemetry, machines send telemetry to persist
  is_human BOOLEAN DEFAULT FALSE,
  is_admin BOOLEAN DEFAULT FALSE,
  is_active BOOLEAN DEFAULT TRUE,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_accounts_username ON auth.accounts(username);

-- tables for each "identity domain": we use MQTT right now, but we plan to support other transports
-- later tables for those identity domains should be defined below

-- create MQTT ACL table
-- Domain A: MQTT-Specific Policies
CREATE TABLE IF NOT EXISTS auth.mqtt_acls (
  id SERIAL PRIMARY KEY,
  account_id UUID NOT NULL REFERENCES auth.accounts(id) ON DELETE CASCADE,
  topic VARCHAR(256) NOT NULL,
  access_level INT NOT NULL CHECK (access_level IN (1, 2, 3)), -- 1=R, 2=W, 3=RW
  created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_mqtt_acls_account ON auth.mqtt_acls(account_id);

-- TODO: future transport auth additions:

-- Domain B: gRPC-Specific Policies
-- CREATE TABLE auth.grpc_policies (
--     id SERIAL PRIMARY KEY,
--     account_id UUID NOT NULL REFERENCES auth.accounts(id) ON DELETE CASCADE,
--     grpc_service VARCHAR(256) NOT NULL, -- e.g. "formulatel.v1.GameTelemetryService"
--     grpc_method VARCHAR(256) NOT NULL,  -- e.g. "StreamLapData"
--     allow_access BOOLEAN DEFAULT TRUE,
--     created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
-- );


-- anything else?

END;