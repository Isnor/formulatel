# this script should only be run a single time and before any schema migrations are run.

-- user and database for Grafana - https://grafana.com/docs/grafana/latest/setup-grafana/configure-grafana/#database
CREATE USER grafana_admin WITH PASSWORD '';
CREATE DATABASE grafana OWNER grafana_admin;

-- database and schema for the telemetry data
CREATE DATABASE formulatel;

CREATE EXTENSION IF NOT EXISTS timescaledb;
CREATE SCHEMA telemetry;
CREATE SCHEMA auth;

-- root org dashboard user - used to configure the datasource to view all orgs' telemetry
CREATE USER grafana_viewer WITH PASSWORD '';
GRANT CONNECT ON DATABASE formulatel TO grafana_viewer;
GRANT USAGE ON SCHEMA telemetry TO grafana_viewer;
GRANT SELECT ON ALL TABLES IN SCHEMA telemetry TO grafana_viewer;
ALTER DEFAULT PRIVILEGES IN SCHEMA telemetry GRANT SELECT ON TABLES TO grafana_viewer;

-- user for the MQTT broker
CREATE USER mosquitto_broker WITH PASSWORD '';
GRANT CONNECT ON DATABASE formulatel TO mosquitto_broker;
GRANT USAGE ON SCHEMA auth TO mosquitto_broker;
GRANT SELECT ON ALL TABLES IN SCHEMA auth TO mosquitto_broker;
ALTER DEFAULT PRIVILEGES IN SCHEMA auth GRANT SELECT ON TABLES TO mosquitto_broker;

-- user for persist
CREATE ROLE formulatel_persist WITH LOGIN PASSWORD '';
GRANT CONNECT ON DATABASE formulatel TO formulatel_persist;
GRANT USAGE ON SCHEMA telemetry TO formulatel_persist;
GRANT INSERT, SELECT ON ALL TABLES IN SCHEMA telemetry TO formulatel_persist;
ALTER DEFAULT PRIVILEGES IN SCHEMA telemetry GRANT INSERT, SELECT ON TABLES TO formulatel_persist;
GRANT USAGE ON ALL SEQUENCES IN SCHEMA telemetry TO formulatel_persist;
ALTER DEFAULT PRIVILEGES IN SCHEMA telemetry GRANT USAGE ON SEQUENCES TO formulatel_persist;