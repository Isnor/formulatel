# AGENTS.md - formulatel Package

This directory contains the core Go module for the Formula Telemetry system.

## Build Instructions

### Prerequisites
- Go 1.25.1+ (toolchain 1.26.1 recommended)
- `protoc` and `protoc-gen-go` for protobuf generation

### Building Binaries

```bash
# Build all binaries (ingest, persist, replay)
cd /home/james/workspace/f1telemetry
make build

# Binaries will be in ./out/
ls -lh ./out/
```

This produces three binaries:
- `./out/ingest` - Reads UDP telemetry from F1 23 and publishes to MQTT
- `./out/persist` - Subscribes to MQTT and writes to TimescaleDB
- `./out/replay` - Replays captured packets for development

### Running Individual Binaries

```bash
# Run ingest locally (requires UDP port forwarding from game)
./out/ingest

# Run persist (requires environment variables)
TIMESCALE_DSN="postgres://user:pass@localhost:5432/postgres?sslmode=disable" \
MQTT_BROKER="tcp://localhost:1883" \
./out/persist

# Run replay (for development with captured packets)
mkdir -p captured_packets
./out/replay --read-dir=./captured_packets --write-port=27543
```

### Building from the formulatel Directory

If you want to build just from within the `formulatel` directory:

```bash
# Navigate to parent and build
cd /home/james/workspace/f1telemetry
make build

# Or build manually from formulatel
cd /home/james/workspace/f1telemetry/formulatel
go build -o ../out/ingest ./cmd/ingest
go build -o ../out/persist ./cmd/persist
go build -o ../out/replay ./cmd/replay
```

## Test Instructions

### Running All Tests

```bash
cd /home/james/workspace/f1telemetry
go test ./formulatel/... -v
```

### Database Tests (Require Docker)

Tests using testcontainers require Docker with PostgreSQL support:

```bash
# Run all tests, includes database tests - testcontainers brings up a container for the database tests
go test ./formulatel/internal/timescale/... -v
```

### Running Tests with Coverage

```bash
go test ./formulatel/... -v -coverprofile=coverage.out
go tool cover -html=coverage.out -o coverage.html
```

### Test Containers

The `internal/timescale` package uses testcontainers to spin up PostgreSQL for integration tests:

```bash
# List available tests
go test ./formulatel/internal/timescale/... -list=.

# Run specific test
go test ./formulatel/internal/timescale/... -run TestSimpleDBWrites -v
```

## Project Structure

```
formulatel/
├── cmd/
│   ├── ingest/       # UDP reader -> MQTT publisher
│   ├── persist/      # MQTT subscriber -> TimescaleDB writer
│   └── replay/       # Packet replayer for development
├── f123/             # F1 23 packet parsing
│   ├── model.go      # Packet structures
│   └── f123.go       # PacketReader, PacketTransformer
├── internal/
│   ├── genproto/     # Generated protobuf code
│   ├── mqttutil/     # MQTT connection utilities
│   └── timescale/    # TimescaleDB persistence layer
├── formulatel.go     # Core interfaces
└── AGENTS.md         # This file
```

## Environment Variables

For the `persist` binary, these environment variables are required:

```bash
TIMESCALE_DSN="postgres://user:pass@host:5432/db?sslmode=disable"
MQTT_BROKER="tcp://broker:1883"
MQTT_PREFIX="formulatel"        # default
BATCH_SIZE="500"                # default
FLUSH_INTERVAL="200"            # milliseconds, default
```

## MQTT Configuration

Both `ingest` and `persist` use MQTT v3 protocol (required for Grafana Live).

Default broker: `tcp://localhost:1883`

Topics:
- Publish: `formulatel/vehicledata/f123`
- Publish: `formulatel/motiondata/f123`
- Subscribe: `formulatel/+/f123` (wildcard)

## Protocol Buffers

Protobufs are generated from files in `protobuf/*.proto`.

```bash
# Regenerate protobufs
make proto
```

This requires `protoc` and `protoc-gen-go` to be installed.

## Debugging

Enable debug logging in `ingest`:

```bash
# In ingest/main.go, uncomment:
// mqtt.ERROR = slog.NewLogLogger(slog.NewTextHandler(os.Stderr, nil), slog.LevelError)
```

For persist, set log level via environment:

```bash
LOG_LEVEL=debug ./out/persist
```

## Troubleshooting

### Build errors

```bash
# Clean and rebuild
make clean
make build
```

### Test failures

```bash
# Ensure Docker is running
docker ps

# Check for container conflicts
docker ps -a | grep postgres
```

### UTF8 encoding errors

Ensure `ingest` uses `EmitUnpopulated: false` when marshaling protobufs to JSON.

### "row field count" errors

Always include all 27 columns for vehicle_data and 17 columns for motion_data in `buildRow()`, even if nullable fields are nil.
