load('ext://helm_resource', 'helm_resource', 'helm_repo')
# this repo gives us an easy way to start an MQTT broker, mosquitto
helm_repo("k8s-at-home", resource_name="k8s-at-home_helm_repo", url="https://k8s-at-home.com/charts/", labels=["helm"])
helm_repo("open-telemetry", resource_name="otel_helm_repo", url="https://open-telemetry.github.io/opentelemetry-helm-charts", labels=["helm"])

# TODO: probably remove this and use `tilt up --namespace=`. namespace will need to be created before tilt is run
k8s_yaml("kubernetes/namespace.yml")

# Load Grafana with dashboards via --set-file to pass JSON content directly to Helm
helm_resource("grafana", chart="oci://ghcr.io/grafana-community/helm-charts/grafana", namespace="formulatel", flags=["--values", "./kubernetes/config/grafana-values.yml"], port_forwards="3000", labels=["infra"])
helm_resource("mosquitto", chart="k8s-at-home/mosquitto", namespace="formulatel", port_forwards="1883", labels=["infra"])
helm_resource("otel-ebpf-auto", chart="open-telemetry/opentelemetry-ebpf-instrumentation", namespace="formulatel", flags=["--values", "./kubernetes/config/open-telemetry-ebpf-values.yml"], labels=["infra"])

# set up a postgres instance with timescaleDB
k8s_yaml("kubernetes/datastore.yml")
k8s_resource("timescaledb", port_forwards="5432", labels=["infra"])

# database migrations
docker_build("formulatel/migrate", context=".", dockerfile="migrations.Dockerfile")
k8s_yaml("kubernetes/migrate-job.yml")
k8s_resource("db-migrations", labels=["infra"])

# build, run, and reload ingest (outside of the k8s cluster)
local_resource(
  "formulatel_ingest",
  cmd="make build",
  serve_cmd="./out/ingest",
  # trigger_mode=TRIGGER_MODE_MANUAL,
  deps=[
    "./formulatel/cmd/ingest",
    "./formulatel/f123",
    "./formulatel/internal/mqttutil",
    "./protobuf",
  ],
  env={
    "LOG_LEVEL": "info",
  },
  labels=["formulatel"]
)

# build and run the persist service
docker_build("formulatel/persist", context="./formulatel", dockerfile="formulatel/persist.Dockerfile")
k8s_yaml("kubernetes/persist.yml")
k8s_resource("persist", labels=["formulatel"])