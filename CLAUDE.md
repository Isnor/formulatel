# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Formula Telemetry is an open-source sim-racing telemetry system that collects, transforms, and visualizes racing sim data. It reads telemetry from games (currently F1 23), converts it to a title-agnostic format, and visualizes it in Grafana with live charting.

## Development Commands

### Local Development (with Tilt)
- `tilt up` - Start services in Kubernetes with live reload. Runs `ingest` locally (due to UDP forwarding issues) and deploys Grafana + MQTT broker to k8s.
- `make k8s-cluster` - Create a local Kubernetes cluster using kind and ctlptl

### Building and Running
- `make build` - Build binaries for `ingest`, `persist`, and `replay` to `./out/`
- `./out/ingest` - Run the ingest service locally (reads UDP telemetry from F1 23)
- `./out/persist` - Run the persist service (writes to TimescaleDB)
- `./out/replay` - Replay captured packets for development (capture must be enabled during ingest)

### Go Dependencies
- Go version: 1.25.1
- Core dependencies:
  - `github.com/eclipse/paho.mqtt.golang v1.5.1` - MQTT client (v3 protocol)
  - `google.golang.org/protobuf v1.33.0` - Protocol buffers
  - `github.com/jackc/pgx/v5 v5.9.2` - PostgreSQL driver
  - `github.com/kelseyhightower/envconfig v1.4.0` - Environment variable binding

## Architecture

The system is an ETL pipeline with three main components:

### ingest (`formulatel/cmd/ingest/`)
- Reads UDP packets from F1 23 game on port 27543
- Parses binary packets using `formulatel/f123/` package
- Transforms to standard `GameTelemetry` protobuf format
- Publishes to MQTT topics (v3 protocol, mosquitto broker)
- Uses `mqttutil.NewMQTTv3Connection()` from `internal/mqttutil`

### persist (`formulatel/cmd/persist/`)
- Subscribes to `formulatel/+/f123` MQTT wild-card topic
- Batches messages and writes to TimescaleDB using `pgx.CopyFrom`
- Dual-trigger flush (batch size + time interval)
- Graceful shutdown with buffer drain

### Visualization
- Grafana with `grafana-mqtt-datasource` plugin for live charting
- MQTT broker (mosquitto) runs in k8s on port 1883
- Grafana runs in k8s on port 3000
- **MQTT v3 protocol** - Required by Grafana Live (v5 not supported)

## Key Packages

- `formulatel/f123/` - F1 23 specific packet parsing (`F123PacketReader`, `F123PacketTransformer`)
- `formulatel/formulatel.go` - Core interfaces (`TelemetryReader`, `TelemetryPersistor`, `FormulaTelPersist`)
- `formulatel/internal/genproto/` - Generated protobuf code for telemetry format
- `formulatel/internal/mqttutil/` - Shared MQTT connection utilities
- `formulatel/internal/timescale/` - TimescaleDB persistence layer

### TimescaleDB Persistence (`internal/timescale/`)

- **`config.go`** - Environment variable binding via `envconfig`
  - `TIMESCALE_DSN`: PostgreSQL connection string
  - `MQTT_BROKER`: MQTT broker URL
  - `MQTT_PREFIX`: MQTT topic prefix (default: `formulatel`)
  - `BATCH_SIZE`: Max rows per batch (default: 500)
  - `FLUSH_INTERVAL`: Flush interval (default: 200ms in config, 10s in code - use 200ms for production)

- **`schema.go`** - TimescaleDB hypertable DDL
  - `VehicleDataSchema`: Vehicle telemetry with 16 tire sensor columns
  - `MotionDataSchema`: Physics data (position, velocity, g-force, rotation)
  - `EnsureSchema()`: Creates hypertables with `timescaledb.hypertable`

- **`batcher.go`** - Batched writes with dual-trigger flush
  - `TableBatcher`: Owns channel, mutex-guarded buffer, ticker
  - `BatchRouter`: Routes messages to vehicle_data and motion_data batchers
  - Uses `pgx.CopyFrom` for efficient COPY protocol inserts
  - Flushes when batch size reached OR flush interval elapsed

## Packet Definitions (formulatel/f123/)

### All 12 Packet Types (from F1 23 spec)

1. **CarMotionPacket** - Motion data for player's car (position, velocity, g-force, rotation)
2. **SessionPacket** - Session information (track, time left)
3. **LapDataPacket** - Lap times for all cars in session
4. **EventPacket** - Notable events during session
5. **ParticipantsPacket** - List of participants (multiplayer)
6. **CarSetupsPacket** - Car setup configurations
7. **CarTelemetryPacket** - Telemetry data for all cars
8. **CarStatusPacket** - Status data for all cars
9. **FinalClassificationPacket** - Final race classification
10. **LobbyInfoPacket** - Multiplayer lobby information
11. **CarDamagePacket** - Damage status for all cars
12. **SessionHistoryPacket** - Lap and tyre data for session
13. **TyreSetsPacket** - Extended tyre set data
14. **MotionExPacket** - Extended motion data for player car

### Currently Implemented (2 of 12)
- **CarTelemetryPacket** (22 bytes) - Vehicle telemetry
- **CarMotionPacket** (22 bytes) - Motion/physics data

### Packet Sizes and Throughput
- CarTelemetryData: ~159 KB/s at 120 Hz
- CarMotionData: ~159 KB/s at 120 Hz
- Total worst case: ~480 packets/s, over half MB per second

### Field Mappings

#### CarTelemetryData fields
- `Speed`: Vehicle speed (km/h)
- `Throttle`: 0.0-1.0
- `Steer`: -1.0 to 1.0
- `Brake`: 0.0-1.0
- `Clutch`: 0-100
- `Gear`: 1-8, N=0, R=-1
- `EngineRPM`: Engine revolutions per minute
- `DRS`: 0=off, 1=on
- `RevLightsPercent`: LED percentage
- `RevLightsBitValue`: LED bits (0-14)
- `BrakesTemperature[4]`: FrontLeft, FrontRight, BackLeft, BackRight (celsius)
- `TyresSurfaceTemperature[4]`: Surface temp (celsius)
- `TyresInnerTemperature[4]`: Inner temp (celsius)
- `EngineTemperature`: Engine temp (celsius)
- `TyresPressure[4]`: Tire pressure (PSI)
- `SurfaceType[4]`: Driving surface type

#### CarMotionData fields
- `WorldPosition[XYZ]`: World space position (metres)
- `WorldVelocity[XYZ]`: World space velocity (m/s)
- `WorldForwardDir[XYZ]`: Forward direction (normalized)
- `WorldRightDir[XYZ]`: Right direction (normalized)
- `GForceLateral`: Lateral g-force
- `GForceLongitudinal`: Longitudinal g-force
- `GForceVertical`: Vertical g-force
- `Yaw`, `Pitch`, `Roll`: Rotation angles (radians)

## Data Flow

```
F1 23 Game (UDP 27543)
    ↓
ingest (localhost)
    ↓
MQTT Topics:
  - formulatel/vehicledata/f123
  - formulatel/motiondata/f123
    ↓
persist (subscribes to formulatel/+/f123)
    ↓
TimescaleDB (vehicle_data, motion_data hypertables)
    ↓
Grafana (live via MQTT, historical via PostgreSQL)
```

## Environment Variables

### Ingest
- `LOG_LEVEL`: Logging level (default: info)
- `MQTT_BROKER`: MQTT broker URL (default: tcp://localhost:1883)
- `CAPTURE`: Enable packet capture to captured_packets/ (default: false)

### Persist
- `TIMESCALE_DSN`: PostgreSQL connection string (required)
- `MQTT_BROKER`: MQTT broker URL (required)
- `MQTT_PREFIX`: MQTT topic prefix (default: formulatel)
- `BATCH_SIZE`: Max rows per batch (default: 500)
- `FLUSH_INTERVAL`: Flush interval in milliseconds (default: 200)

## MQTT Topics

Data is published to MQTT topics by telemetry type. The Grafana datasource connects to `tcp://mosquitto:1883`.

**Publish topics** (from `ingest`):
- `formulatel/vehicledata/f123` - Vehicle telemetry (speed, throttle, steering, brake, RPM, gear, etc.)
- `formulatel/motiondata/f123` - Motion/physics data (position, velocity, g-force, angles, etc.)

**Subscribe topic** (by `persist` and `grafana_plugin`):
- `formulatel/+/f123` - Wild-card subscription for all telemetry types

Data is published as JSON using protojson marshaling with `EmitDefaultValues: true` and `EmitUnpopulated: false`.
- `EmitDefaultValues: true` ensures zero values are included (critical for live dashboard to display all metrics)
- `EmitUnpopulated: false` prevents null bytes (0x00) that cause PostgreSQL UTF8 encoding errors

## Database Schema

### vehicle_data (hypertable) - 27 columns
- `time TIMESTAMPTZ` - Timestamp
- `session_id TEXT` - Session identifier
- `user_id TEXT` - Player/car identifier
- `title INTEGER` - Game title enum
- `speed INTEGER` - Speed (km/h)
- `rpm INTEGER` - Engine RPM
- `throttle/brake/steering REAL` - Input values (0-1)
- `gear INTEGER` - Current gear
- `engine_temperature INTEGER` - Engine temp (celsius)
- `fl_brake_temp, fl_inner_temp, fl_surface_temp, fl_pressure` - Front-left tire
- `fr_brake_temp, fr_inner_temp, fr_surface_temp, fr_pressure` - Front-right tire
- `bl_brake_temp, bl_inner_temp, bl_surface_temp, bl_pressure` - Back-left tire
- `br_brake_temp, br_inner_temp, br_surface_temp, br_pressure` - Back-right tire

### motion_data (hypertable) - 17 columns
- `time TIMESTAMPTZ` - Timestamp
- `session_id TEXT` - Session identifier
- `user_id TEXT` - Player/car identifier
- `title INTEGER` - Game title enum
- `position_x, position_y, position_z` - World position (metres)
- `velocity_x, velocity_y, velocity_z` - World velocity (m/s)
- `gforce_lateral, gforce_longitudinal, gforce_vertical` - G-force components
- `yaw, pitch, roll` - Rotation angles (radians)

## Packet Capture

For development, `ingest` has a `capture` flag that can write packets to `captured_packets/` directory for later replay with `./out/replay`.
**Note:** Packet capture is currently disabled by default. Enable by setting `capture: true` in the `F123PacketTransformer`.

## Migration Files

Migrations are stored in `migrations/` directory using [golang-migrate](https://github.com/golang-migrate/migrate).

- `20260514001730_create_vehicle_data_table.up.sql` - Creates vehicle_data hypertable (27 columns)
- `20260514001855_create_motion_data_table.up.sql` - Creates motion_data hypertable (17 columns)

Run migrations with `make migrate`.

## Kubernetes Manifests

Located in `kubernetes/` directory:

- `namespace.yml` - Creates formulatel namespace
- `datastore.yml` - TimescaleDB StatefulSet with PVC
- `persist.yml` - Persist service deployment
- `migrate-job.yml` - Database migration job
- `config/grafana-values.yml` - Helm values for Grafana
- `config/live_dash_v2.json` - Grafana live dashboard JSON

## Grafana Dashboard

The live dashboard is configured in `kubernetes/config/live_dash_v2.json` and includes:

- **Vehicle Data Panels** (MQTT):
  - Throttle (gradient bargauge)
  - Steering (gradient bargauge)
  - Brake (gradient bargauge)
  - Speed (gauge with unit: velocitykmh)
  - Engine Temp (timeseries: celsius)
  - RPM (gauge: rotrpm)
  - Gear (stat: short)

- **Motion Data Panels** (MQTT):
  - Position (xychart: vehicle trajectory)
  - Pitch Angle (gradient bargauge: radian)
  - Roll Angle (gradient bargauge: radian)
  - g-force (xychart: lateral vs longitudinal)

## Adding Support for a New Title

The system uses a **package-per-title** design pattern. Each racing sim has its own package that handles title-specific parsing and normalization.

### Steps to Add a New Title

1. Create a new package: `formulatel/<titlename>/`
2. Define the title's packet structures in `model.go`
3. Implement packet types enum matching the game's packet types
4. Implement `TitlePacketReader` to read UDP packets
5. Implement `TitlePacketTransformer` with:
   - `Consume()` method to process packets
   - `Route()` method to handle different packet types
   - Normalization logic mapping title fields to `pb.VehicleData` and `pb.MotionData`
6. Add a new `GameTitle` enum value in `protobuf/telemetry.proto`
7. Update `ingest/main.go` to use the new transformer
8. Publish to a title-specific MQTT topic (e.g., `formulatel/vehicledata/<titlename>`)
9. Add migration files for the new title's schema

### Package Structure Example

```
formulatel/<titlename>/
  model.go       - Packet structures and packet type enum
  <title>.go     - PacketReader, PacketTransformer, Route() implementation
```

### Normalization Pattern

The transformer performs two key operations:

1. **Parsing**: Reads the title's binary format using `ReadBin[[N]PacketType](file:///path/to/model.go#LXX-LYY)`
2. **Normalization**: Maps to standard protobuf schema (e.g., `Speed: uint32(parsed.Speed)`)

This happens in the `Route()` method. Only implemented packet types are handled.

### Key Design Decisions

- **No separate `TelemetryNormalizer` interface needed** - The transformer handles both parsing and normalization, which naturally belong together
- **Channel-based output** - Transformers output to channels, making them independent of the destination (MQTT, database, etc.)
- **Title-agnostic protobuf schema** - All titles normalize to the same `GameTelemetry` format, enabling single-dashboard visualization across titles

## Troubleshooting

### Error: "row field count is X, expected Y"
**Cause**: `buildRow()` function only added columns when `vd.Tires != nil`, resulting in mismatched column counts
**Fix**: Always include all 27 columns for vehicle_data and 17 columns for motion_data, even if nullable fields are nil
**Location**: `formulatel/internal/timescale/batcher.go:buildRow()`

### Error: "invalid byte sequence for encoding UTF8: 0x00"
**Cause**: `protojson.MarshalOptions{EmitUnpopulated: true}` was outputting null bytes (0x00) which are invalid in UTF8
**Fix**: Changed to `EmitUnpopulated: false` while keeping `EmitDefaultValues: true` to preserve zero values
**Location**: `formulatel/cmd/ingest/mqtt_v3.go:StartMQTTv3Publisher()`

### Error: "unexpected EOF in COPY data"
**Cause**: Result of the above two errors - pgx receives malformed rows and aborts the COPY operation
**Fix**: Both fixes above resolve this error

### Persist Service
- Verify TimescaleDB connection string
- Check that schema migrations have run
- Monitor batcher flush rates
- Check PostgreSQL logs for encoding errors
- Verify column counts match schema (27 for vehicle_data, 17 for motion_data)

### Grafana
- Ensure MQTT datasource is configured
- Verify live dashboard JSON is loaded
- Check plugin version compatibility

## Problems and Limitations

- **MQTT v3 only** - Grafana Live requires v3; v5 not supported
- **Partial packet support** - Only 2 of 12 packet types implemented (CarTelemetryPacket, CarMotionPacket)
- **No packet deduplication** - Duplicate packets may be processed
- **Sequential persistence** - `FormulaTelPersist.Run()` processes telemetry sequentially; batching is done in persist service
- **UDP port forwarding** - Ingest must run locally due to k8s UDP forwarding limitations
- **UTF8 encoding** - Protobuf JSON marshaling must use `EmitUnpopulated: false` to avoid null bytes

## Database Backup Strategy

### Retention and Backup Options

#### 1. TimescaleDB Automatic Retention (Simplest)
```sql
-- Set chunk retention policy to keep data for 7 days
SELECT add_retention_policy('vehicle_data', INTERVAL 7 days);
SELECT add_retention_policy('motion_data', INTERVAL 7 days);
```

#### 2. pg_dump Daily + S3
Use PostgreSQL's built-in backup tool for full backups.

#### 3. TimescaleDB Timeshift + S3 (Recommended)
Timescale provides `timeshift` for time-travel backups.

```yaml
# k8s CronJob for automated S3 backups
apiVersion: batch/v1
kind: CronJob
metadata:
  name: timescale-backup
  namespace: formulatel
spec:
  schedule: "0 2 * * *"  # Daily at 2 AM
  jobTemplate:
    spec:
      template:
        spec:
          containers:
          - name: backup
            image: timescale/timescaledb-ha:pg18
            command: ["/bin/sh", "-c"]
            args:
            - |
              timeshift -n -t daily -f /backup \
                --timescaledb-user=postgres \
                --pguser=postgres --pgpassword=postgres \
                --dbname=postgres \
                --timescaledb-datasource=timescaledb:5432
            env:
            - name: AWS_ACCESS_KEY_ID
              valueFrom:
                secretKeyRef:
                  name: aws-creds
                  key: access-key-id
            - name: AWS_SECRET_ACCESS_KEY
              valueFrom:
                secretKeyRef:
                  name: aws-creds
                  key: secret-access-key
          restartPolicy: Never
```

#### 4. WAL Archiving (Point-in-Time Recovery)
Enable WAL archiving for point-in-time recovery:
```sql
ALTER SYSTEM SET wal_level = 'replica';
ALTER SYSTEM SET archive_mode = on;
ALTER SYSTEM SET archive_command = 'pg_archiveWal %p /tmp/wal/%f';
```

### Recommended Hybrid Approach
1. Keep recent data hot (last 30 days) in main hypertables
2. Timeshift backups to S3 every 6 hours
3. WAL archiving to S3 for point-in-time recovery
4. Retention policy dropping chunks older than 90 days

## Goals

Completed:
- [x] grafana dashboards reading from k8s cluster
- [x] chart telemetry data
- [x] realtime charting with Grafana Live
- [x] MQTT v3 protocol support

Future:
- [ ] Generic racing telemetry <-> metric conversion
- [ ] Build dashboard for interesting telemetry data
- [ ] Insights: braking point detection, racing line optimization
- [ ] Tire wear prediction
- [ ] eBPF packet inspection and routing
