# AGENTS.md - formulatel Package

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Formula Telemetry is an open-source sim-racing telemetry system that collects, transforms, and visualizes racing sim data. It reads telemetry from games (currently F1 23), converts it to a title-agnostic format, and visualizes it in Grafana with live charting.

## Agents & Memory

Look at the project-level .claude directory for a README.md, `agents/` directory for agent definitions, a `memory/` directory for general-use memory files, and an `agent-memory/` directory for agent-specific memory files.

## Development Commands

### Local Development
- `make proto` - generate the protocol buffer code from protobuf/telemetry.proto. Must be run from the root of the repository
- `go test -v ./...` - run the unit tests. Must be run from the `formulatel` directory.

### Building and Running
- `make build` - Build binaries for `ingest`, `persist`, and `replay` to `./out/`. Must be run from the root of the repository.
- `./out/ingest` - Run the ingest service locally (reads UDP telemetry from F1 23)
- `./out/persist` - Run the persist service (writes to TimescaleDB)
- `./out/replay` - Replay captured packets for development (capture must be enabled during ingest)

### Go Dependencies
- Go version: 1.25.1
- Core dependencies:
  - `github.com/eclipse/paho.mqtt.golang v1.5.1` - MQTT client (v3 protocol)
  - `google.golang.org/protobuf v1.33.0` - Protocol buffers
  - `github.com/jackc/pgx/v5 v5.9.2` - PostgreSQL driver

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

- **`batcher.go`** - Batched writes with dual-trigger flush
  - `TableBatcher`: Owns channel, mutex-guarded buffer, ticker
  - `BatchRouter`: Routes messages to vehicle_data, motion_data, session_lap_data, and live_lap_data batchers
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

### Currently Implemented (4 of 12)
- **CarTelemetryPacket** (22 bytes) - Vehicle telemetry
- **CarMotionPacket** (22 bytes) - Motion/physics data
- **LapDataPacket** - Live/current lap data (writes to live_lap_data)
- **SessionHistoryPacket** - Complete historic lap data (writes to session_lap_data)

### Packet Sizes and Throughput
- CarTelemetryData: ~159 KB/s at 120 Hz
- CarMotionData: ~159 KB/s at 120 Hz
- Total worst case: ~480 packets/s, over half MB per second

## Data Flow

```
F1 23 Game (UDP 27543)
    ↓
ingest (localhost)
    ↓
MQTT Topics (by telemetry type):
  - formulatel/vehicledata/f123
  - formulatel/motiondata/f123
  - formulatel/currentlapdata/f123
  - formulatel/lapdata/f123
    ↓
persist (subscribes to formulatel/+/f123)
    ↓
TimescaleDB (vehicle_data, motion_data, live_lap_data, session_lap_data)
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

Data is published to MQTT topics by telemetry type. Each data type has its own topic.
The Grafana datasource connects to `tcp://mosquitto:1883`.
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

### live_lap_data (hypertable) - Current live lap tracking
- `time TIMESTAMPTZ` - Timestamp
- `session_id TEXT` - Session identifier
- `user_id TEXT` - Player/car identifier
- `title INTEGER` - Game title enum
- `lap_num INTEGER` - Current lap number
- `current_lap_time INTEGER` - Current lap time (ms)
- `sector INTEGER` - Current sector (0-3)
- `sector1_time, sector2_time` - Sector times (ms)
- `delta_to_car_in_front` - Time gap to car in front (ms)
- `delta_to_race_leader` - Time gap to race leader (ms)
- `lap_distance, total_distance` - Distance metrics (meters)

### session_lap_data - Complete historic lap data
- `session_id TEXT` - Session identifier
- `user_id TEXT` - Player/car identifier
- `title INTEGER` - Game title enum
- `lap_num INTEGER` - Lap number
- `lap_time INTERVAL` - Complete lap time
- `sector1_time, sector2_time, sector3_time` - Sector times (INTERVAL)
- `lap_valid` - Whether the lap was valid (boolean)
- `sector1_valid, sector2_valid, sector3_valid` - Sector validity flags
- **Unique constraint**: `unique_lap` on (session_id, user_id, lap_num)

## Packet Capture

For development, `ingest` has a `capture` flag that can write packets to `captured_packets/` directory for later replay with `./out/replay`.
**Note:** Packet capture is currently disabled by default. Enable by setting `capture: true` in the `F123PacketTransformer`.

## Adding New Data Types

To add support for new data types, follow these steps:

### 1. Define the protocol buffer
Add the new message type to `protobuf/telemetry.proto`. The user needs to specify the structure of the data. If the user needs help defining the schema for the new data, use context7 to try to find documentation for existing racing telemetry data models.

### 2. Model the native data
Update a model package under `formulatel/<title>/` that:
- Defines packet structures matching the source format
- Update `F123PacketTransformer` with:
  - `Route()` method to handle the new data type types and normalize to `GameTelemetry`

### 3. Push to MQTT topic
The transformer publishes to a topic like `formulatel/<datatype>/<title>` for Grafana and persist to consume.

### 4. Create database migration
Add migration files in `migrations/` directory to create a new hypertable for the data type.

### 5. Update persist service
Configure `persist` to write to the new database table via `BatchRouter`.

## Migration Files

Migrations are stored in `migrations/` directory using golang-migrate.
Run with `make migrate`.

## Kubernetes Manifests

Located in `kubernetes/` directory:

- `namespace.yml` - Creates formulatel namespace
- `datastore.yml` - TimescaleDB StatefulSet with PVC
- `persist.yml` - Persist service deployment
- `migrate-job.yml` - Database migration job
- `config/grafana-values.yml` - Helm values for Grafana
- `config/live_dash_v2.json` - Grafana live dashboard JSON

## Grafana Dashboard

Dashboards are defined in `kubernetes/config/dashboards`. There is one for live data and one for historic data. Create them with `make`.

## Adding Support for a New Title

The system uses a **package-per-title** design pattern. Each racing sim has its own package that handles title-specific parsing and normalization. Adding a new title involves:

1) reading data from the title using whatever transport that title provides. For example, UDP packets or shared memory.
2) modeling the data from the title's native format into structured data
3) converting that structured data into the GameTelemetry format

See ["Adding New Data Types"](#adding-new-data-types) for detailed steps.

### Key Design Decisions

- **No separate `TelemetryNormalizer` interface needed** - The transformer handles both parsing and normalization, which naturally belong together
- **Channel-based output** - Transformers output to channels, making them independent of the destination (MQTT, database, etc.)
- **Title-agnostic protobuf schema** - All titles normalize to the same `GameTelemetry` format, enabling single-dashboard visualization across titles

## Troubleshooting

### Persist Service
- Verify TimescaleDB connection string
- Check that schema migrations have run
- Monitor batcher flush rates
- Check PostgreSQL logs for encoding errors

### Grafana
- Ensure MQTT datasource is configured
- Verify live dashboard JSON is loaded
- Check plugin version compatibility

## Problems and Limitations

- **MQTT v3 only** - Grafana Live requires v3; v5 not supported
- **Packet support** - Multiple packet types now implemented: CarTelemetryPacket, CarMotionPacket, LapDataPacket (session history), Latest lap data tracking
- **No packet deduplication** - Duplicate packets may be processed
- **Sequential persistence** - `FormulaTelPersist.Run()` processes telemetry sequentially; batching is done in persist service
- **UDP port forwarding** - Ingest must run locally due to k8s UDP forwarding limitations
- **UTF8 encoding** - Protobuf JSON marshaling must use `EmitUnpopulated: false` to avoid null bytes

## Lap Data Persistence

The system now supports two additional tables for lap data:

### 1. `live_lap_data` - Current Live Lap Tracking
- **Purpose**: Stores the current/incomplete lap data for real-time visualization
- **Hypertable**: Yes (chunked by day, compressed)
- **Columns**: time, session_id, user_id, title, lap_num, current_lap_time, sector, sector1_time, sector2_time, delta_to_car_in_front, delta_to_race_leader, lap_distance, total_distance
- **Batcher**: Uses `buildCurrentLapDataRow()` to convert protobuf `CurrentLapData` to row map
- **Persistence**: Written via dedicated batcher when current lap data is received

### 2. `session_lap_data` - Historic Lap Data
- **Purpose**: Stores complete lap data for historical analysis and session review
- **Columns**: session_id, user_id, title, lap_num, lap_time (INTERVAL), sector1_time (INTERVAL), sector2_time (INTERVAL), sector3_time (INTERVAL), lap_valid, sector1_valid, sector2_valid, sector3_valid
- **Unique constraint**: `unique_lap` on (session_id, user_id, lap_num) to prevent duplicate historic lap entries
- **Batcher**: Uses `WriteLapRow()` for direct INSERT with ON CONFLICT DO NOTHING
- **Persistence**: Written via dedicated batcher when session history packets are received

### Latest Lap Data Tracking
- **Purpose**: Prevents duplicate historic lap entries by tracking the latest lap number received per session/user
- **Type**: `LatestLapData` struct with thread-safe map (sessionID.userID -> lapNum)
- **File**: `formulatel/f123/latest_lapdata.go`
- **Usage**: `LatestLapData.Get/Set()` called during `normalizeSessionHistoryData()` to deduplicate completed laps

### Protocol Changes
- Lap data now writes to two separate tables based on packet type
- Protobuf schema distinguishes between `CurrentLapData` and `HistoricLapData`
- Session history packets with complete lap data are stored for later analysis

### Testing
The new batcher logic is tested in `formulatel/internal/timescale/timescale_test.go`:
- `TestBatchRouter`: Tests all four batchers (vehicle, motion, session_lap_data, live_lap_data)
- `TestDuplicateLapTimes`: Verifies ON CONFLICT DO NOTHING prevents duplicate historic lap entries

### MQTT Topics
Data is published to topics for current lap data and session history lap data.

## Goals

Completed:
- [x] grafana dashboards reading from k8s cluster
- [x] chart telemetry data
- [x] realtime charting with Grafana Live
- [x] MQTT v3 protocol support
- [x] Build dashboard for interesting telemetry data
- [x] Live lap data persistence
- [x] Historic lap data persistence with duplicate prevention

Future:
- [ ] Insights: braking point detection, racing line optimization
- [ ] Tire wear prediction
- [ ] eBPF packet inspection and routing