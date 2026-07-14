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

We use an "init container" pattern (e.g., using `envsubst`) for the Mosquitto configuration. This allows us to inject secrets directly into the config files at runtime without requiring complex local templating in Helm, solving the issue of overlapping environment variables. See the [Mosquitto Configuration section](#Mosquitto Configuration) for more detailed information on the `/etc/mosquitto/mosquitto.conf` file.

## Configuration Values

The available parameters can be found in `values.yaml`. Key sections include:

- **global**: Core settings like the target namespace.
- **persist**: Deployment details for the backend service (replica count, image tags).
- **migrate**: Configuration for the migration runner.
- **db**: Connection settings for the database.
- **grafana_ingress / mqtt_ingress**: Toggleable logic to expose services via Ingress controllers.
- **app-template**: Custom configuration for the Mosquitro broker, including persistence and port mapping.

Refer to `values.yaml` for a full list of available overrides.

### Mosquitto Configuration

The `mosquitto` deployment reads its `mosquitto.conf` from a shared volume in order to support the sensitive values that need to be written to it when `auth` is enabled. Our deployment starts an init container to read secrets from kubernetes and uses `envsubst` to write the secrets to `mosquitto.conf` in the shared volume. This means that if you do want to enable auth, you need to define the secrets for the init container and override the entire ConfigMap in the `values.yaml`:

```yaml
# override the init container's environment with your secrets
app-template:
  controllers:
    mosquitto: # don't change this name
      initContainers:
        config-renderer: # don't change this name
          env:
            - name: MOSQUITTO_DB_USERNAME
              valueFrom:
                secretKeyRef:
                  name: formulatel-db-user-mosquitto-broker
                  key: username
            - name: MOSQUITTO_DB_PASSWORD
              valueFrom:
                secretKeyRef:
                  name: formulatel-db-user-mosquitto-broker
                  key: password
  configMaps:
    mosquitto-template: # don't change this
      enabled: true
      data:
        # don't change the name of mosquitto.conf.tmpl either, unless you change the
        # config-renderer.
        mosquitto.conf.tmpl: |
          persistence true
          persistence_location /mosquitto/data/
          autosave_interval 1800
          per_listener_settings true

          listener 1883 0.0.0.0
          protocol mqtt
          allow_anonymous true

          listener 9001 0.0.0.0
          protocol websockets
          allow_anonymous false

          # Plugin Binding
          auth_plugin /mosquitto/go-auth.so
          auth_opt_log_level debug
          auth_opt_backends postgres

          # Postgres Integration
          auth_opt_pg_host 127.0.0.1
          auth_opt_pg_port 5432
          auth_opt_pg_dbname postgres
          # these two values get replaced by `envsubst` and must match the env defined above
          auth_opt_pg_user ${MOSQUITTO_DB_USERNAME}
          auth_opt_pg_password ${MOSQUITTO_DB_PASSWORD}
          auth_opt_pg_sslmode disable

          # Queries
          auth_opt_pg_userquery SELECT password_hash FROM auth.accounts WHERE username = $1 LIMIT 1
          auth_opt_pg_superquery SELECT COUNT(*) FROM auth.accounts WHERE username = $1 AND is_admin = true
          auth_opt_pg_aclquery SELECT topic FROM auth.mqtt_acls JOIN auth.accounts ON mqtt_acls.account_id=accounts.id WHERE username = $1 AND (access_level >= $2)
          auth_opt_hasher bcrypt
```
