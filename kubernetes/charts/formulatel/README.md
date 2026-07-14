# Formulatel Helm Chart

This chart manages the deployment of the `formulatel` system components.

## What is deployed

The following components are provisioned by this chart:

- **formulatel-persist**: The core persistence service that consumes telemetry from MQTT and writes it to TimescaleDB.
- **Database Migrations**: Automated execution of SQL scripts to manage the database schema (via `timestagedb-migrations`).
- **Grafana**: A visualization dashboard for real-time and historical racing data analysis.
- **Mosquitto**: An MQTT broker used as a message bus between the ingestion and persistence layers.

## Design Decisions & Implementation Notes

The chart is structured with several specific design choices:

### Modular Strategy
We use the `app-template` library to manage common component patterns (like Mosquitto). This allows us to maintain cleaner template logic while sharing boilerplate for configuration management.

### Logic Separation
The chart separates concerns between backend services and infrastructure requirements. For example, some components like `database_secret_name` are left blank in the default values because they are intended to be populated by environment-specific overrides or during the deployment pipeline.

### Config Rendering
We use an "init container" pattern (e.g., using `envsubst`) for the Mosquitto configuration. This allows us to inject secrets directly into the config files at runtime without requiring complex local templating in Helm, solving the issue of overlapping environment variables.

## Configuration Values

The available parameters can be found in `values.yaml`. Key sections include:

- **global**: Core settings like the target namespace.
- **persist**: Deployment details for the backend service (replica count, image tags).
- **migrate**: Configuration for the migration runner.
- **db**: Connection and security settings for the database.
- **grafana_ingress / mqtt_ingress**: Toggleable logic to expose services via Ingress controllers.
- **app-template**: Custom configuration for the Mosquitro broker, including persistence and port mapping.

Refer to `values.yaml` for a full list of available overrides.
