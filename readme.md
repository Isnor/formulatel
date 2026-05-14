# Formula Telemetry

Open source sim-racing telemetry

## Overview

This project is about collecting, transforming, and visualizing racing sim telemetry data. The main idea is that it would be neat to create and share telemetry dashboards, and it would be even more neat if we could standardize the telemetry model so that dashboards could be reused by different titles.

```mermaid
---
config:
  theme: redux
  look: neo
---
flowchart LR
  subgraph Sources [Sim Racing Games]
    T1[Title A]
    T2[Title B]
    TN[Title C]
  end
  X[ingest<br/>Receive, Transform, Publish]
  subgraph PubSub [Pub/Sub Topics by Type]
    TypeA[Topic: Motion]
    TypeB[Topic: Weather]
    TypeN[Topic: Tires]
  end
  G[Grafana<br/>Visualization]
  Y[persist<br/>Persistence Layer]
  DB[(Datastore)]
  T1 --> X
  T2 --> X
  TN --> X
  X --> TypeA
  X --> TypeB
  X --> TypeN
  TypeA --> G
  TypeB --> G
  TypeN --> G
  TypeA --> Y
  TypeB --> Y
  TypeN --> Y
  Y --> DB
  DB --> G
```

## Development

### Using tilt

This project uses [Tilt](https://tilt.dev).

* `tilt up` - start the services in Kubernetes and forward ports. It will also rebuild `persist` and `ingest` when code changes are made.
  * `make k8s-cluster` - uses `ctlptl` and `kind` to create a kubernetes cluster

`tilt` will run `ingest` outside of the cluster because of complications with forwarding UDP ports. The kubernetes cluster will contain a Grafana instance and MQTT broker, both of which have their ports forwarded; i.e. 3000 and 1883, respectively.

### Sans k8s

Kubernetes isn't a requirement for developing or running `formulatel`, but it is a convenient way to launch an MQTT broker, a datastore, and Grafana instance. If you have your own Grafana, postgres instance, and MQTT broker to connect to or are interested in writing a non-MQTT `ingest`, you can build and run the `forumlatel` tools locally as long as you have Golang installed:

* `make build`   - builds the protobufs and the binaries for `ingest`, `persist`, and `replay`.
* `./out/ingest` - run the `ingest` binary (assuming you are in the root of the repository) to read telemetry from your game
* `./out/replay` - useful for development; run `ingest` with the `capture` flag set to capture packets from a game to replay later.

### Database migrations

Create new migrations with the [migrate](https://github.com/golang-migrate/migrate) CLI; e.g.:

`migrate create -dir migrations -ext sql create_motion_data_table`

Run the migrations with

`make migrate`

## Goals

- [x] have fun!
- [x] grafana dashboards reading from k8s cluster
- [x] chart telemetry data
- [ ] generic racing telemetry<->metric conversion? It will be a hassle to turn each protobuf into a metric manually, is there a better way?
- [ ] build a dashboard for interesting telemetry data
- [x] realtime charting with something like Grafana Live
- [~] --opensearch-- implemented a PoC with Kafka and didn't get much out of it
- [ ] insights? A lofty goal to be certain, but it'd be cool to alert on realtime data (ideal braking point? racing line? I don't know) or maybe predict when the tires will die or something.
- [ ] eBPF packet inspection and routing - it'd be neat to route packets directly from the syscall using eBPF.


## Architecture

`formulatel` is a pretty straightforward ETL pipeline:
- `ingest` - a service that consumes telemetry data, transforms it into the formulatel format, and sends it to persist.
  - This is the functionality responsible for converting raw telemetry data into the backend format
  - Right now, `ingest` pushes data on to MQTT topics that send asynchronously without any delivery guarantees
- `persist` - a service that subscribes to telemetry topics and persists data to a datastore
- Visualization - Grafana dashboards for the telemetry. If `ingest` is using MQTT, we can do live visualization with a Grafana plugin

## Problems

Some of the problems I've encountered so far include:

- One of my initial goals was to learn about and use open telemetry; I thought that it would make sense to use the otel collector to take in telemetry from a bunch of different sources and export them to a persistent storage for charts after a race and some type of "more live" storage to view during the race. As I learned more and developed some of the PoC work, I realized what I wanted most was a way to view the data live and decided on an easier-to-setup pub/sub queue model instead.
- I really wanted to provide a backend service that a racer could just "send telemetry to"; I wanted to separate the ingestion from the charting so that if somebody wanted to extend it or add support for another game, they would just need to write the data translation. When I implemented this, I realized that the "backend" service I was providing was just recording metrics and that if I want to chart something in real-time I'd want to skip that extra hop and record them directly from the ingestion service.