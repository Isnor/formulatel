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
- `./out/replay` - Replay captured packets for development (capture must be enabled during ingest)

### Protobuf
- Protobuf definitions are in `protobuf/` and generated Go code is in `formulatel/internal/genproto/`
- `telemetry.proto` defines the title-agnostic telemetry format
- `protobuf/f123/` contains F1 23 specific packet definitions

## Architecture

The system is an ETL pipeline with three main components:

### ingest (`formulatel/cmd/ingest/`)
- Reads UDP packets from F1 23 game on port 27543
- Parses binary packets using `formulatel/f123/` package
- Transforms to standard `GameTelemetry` protobuf format
- Publishes to MQTT topics (v3 protocol, mosquitto broker)
- Uses `mqtt_v3.go` for MQTT publishing

### persist (`formulatel/cmd/persist/`)
- Intentionally empty stub - placeholder for future persistence layer
- Intended to subscribe to MQTT topics and persist to a datastore (e.g., database, filesystem)
- Uses `formulatel/formulatel.go` interfaces for pluggable readers/persistors

### Visualization
- Grafana with `grafana-mqtt-datasource` plugin for live charting
- MQTT broker (mosquitto) runs in k8s on port 1883
- Grafana runs in k8s on port 3000
- **MQTT v3 protocol** - Required by Grafana Live (v5 not supported)

## Key Packages

- `formulatel/f123/` - F1 23 specific packet parsing (`F123PacketReader`, `F123PacketTransformer`)
- `formulatel/formulatel.go` - Core interfaces (`TelemetryReader`, `TelemetryPersistor`, `FormulaTelPersist`)
- `formulatel/internal/genproto/` - Generated protobuf code for telemetry format

## Development

### Adding Support for a New Title

The system uses a **package-per-title** design pattern. Each racing sim has its own package that handles title-specific parsing and normalization.

#### Package Structure

Each title package (e.g., `formulatel/f123/`) contains:

1. **Packet Reader** - Reads raw UDP packets from the game
   - Example: `F123PacketReader` reads packets from F1 23 on port 27543
   - Buffers packets into a channel for processing

2. **Packet Transformer** - Parses and normalizes title-specific data
   - Example: `F123PacketTransformer` consumes raw packets and outputs normalized protobuf
   - Implements `Route()` method to handle different packet types
   - Maps title-specific fields to the standard `GameTelemetry` protobuf format

3. **Routing Logic** - Directs different packet types to appropriate handlers
   - Uses packet headers to determine packet type
   - Currently only handles 2 of 12 packet types: CarTelemetryPacket and CarMotionPacket
   - Other packet types (Session, LapData, Event, Participants, Setups, Status, FinalClassification, Lobby, Damage, SessionHistory, TyreSets, MotionEx) are ignored

#### Normalization Pattern

The transformer performs two key operations:

1. **Parsing**: Reads the title's binary format using `ReadBin[[22]CarTelemetryData](file:///home/james/workspace/f1telemetry/formulatel/f123/f123.go#L23-L26)`
2. **Normalization**: Maps to standard protobuf schema (e.g., `Speed: uint32(telemetry.Speed)`)

This happens in the `Route()` method - see [formulatel/f123/f123.go:189-227](formulatel/f123/f123.go#L189-L227). Only two packet types are handled (CarTelemetryPacket and CarMotionPacket).

#### Adding a New Title

To add support for a new racing sim:

1. Create a new package: `formulatel/iracing/` (or similar)
2. Define the title's packet structures in `model.go`
3. Implement `IRacingPacketReader` to read UDP packets
4. Implement `IRacingPacketTransformer` with:
   - `Consume()` method to process packets
   - `Route()` method to handle different packet types
   - Normalization logic mapping title fields to `pb.VehicleData`
5. Add a new `GameTitle` enum value in `protobuf/telemetry.proto`
6. Update `ingest/main.go` to use the new transformer
7. Publish to a title-specific MQTT topic (e.g., `formulatel/vehicledata/iracing`)

#### Key Design Decisions

- **No separate `TelemetryNormalizer` interface needed** - The transformer handles both parsing and normalization, which naturally belong together
- **Channel-based output** - Transformers output to channels, making them independent of the destination (MQTT, database, etc.)
- **Title-agnostic protobuf schema** - All titles normalize to the same `GameTelemetry` format, enabling single-dashboard visualization across titles

## MQTT Topics

Data is published to MQTT topics by telemetry type. The Grafana datasource connects to `tcp://mosquitto:1883`.

Current topics:
- `formulatel/vehicledata/f123` - Vehicle telemetry (speed, throttle, steering, brake, RPM, gear, etc.)
- `formulatel/motiondata/f123` - Motion/physics data (position, velocity, g-force, angles, etc.)

## Packet Capture

For development, `ingest` has a `capture` flag that can write packets to `captured_packets/` directory for later replay with `./out/replay`.
**Note:** Packet capture is currently disabled (the flag exists but is not enabled by default). Enable by setting `capture: true` in the `F123PacketTransformer`.
