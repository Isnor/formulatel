load('ext://helm_resource', 'helm_resource', 'helm_repo')
allow_k8s_contexts('kind-formulatel') # replace this with your local context name

helm_repo("open-telemetry", resource_name="otel_helm_repo", url="https://open-telemetry.github.io/opentelemetry-helm-charts", labels=["helm"])
helm_repo("jaegertracing", resource_name="jager_helm_repo", url="https://jaegertracing.github.io/helm-charts", labels=["helm"])
k8s_yaml("kubernetes/namespace.yml")

helm_resource("otel-obi", chart="open-telemetry/opentelemetry-ebpf-instrumentation", namespace="formulatel", flags=["--values", "./kubernetes/config/local/open-telemetry-obi-values.yml"], labels=["infra"])
helm_resource("otel-collector", chart="open-telemetry/opentelemetry-collector", namespace="formulatel", flags=["--values", "./kubernetes/config/local/open-telemetry-collector-values.yml"], labels=["infra"])
helm_resource("jaeger", chart="jaegertracing/jaeger", namespace="formulatel", labels=["infra"])

# set up a postgres instance with timescaleDB
k8s_yaml("kubernetes/datastore.yml")
k8s_resource("timescaledb", port_forwards="5432", labels=["infra"])


# build, run, and reload ingest (outside of the k8s cluster)
local_resource(
  "formulatel_ingest",
  cmd="make build",
  serve_cmd="./out/ingest",
  trigger_mode=TRIGGER_MODE_MANUAL,
  deps=[
    "./formulatel/cmd/ingest",
    "./formulatel/f123",
    "./formulatel/internal/mqttutil",
    "./protobuf",
  ],
  serve_env={
    # ingest does not listen to log_level
    # "LOG_LEVEL": "debug",
    # "FORMULATEL_CAPTURE_PACKETS": "true",
    "MQTT_BROKER": "ws://localhost:9001",
    "FORMULATEL_USERNAME": "yimmy", # replace with the username returned from the admin CLI
    "FORMULATEL_TENANT_ID": "2", # replace with the tenant ID returned from the admin CLI
    "FORMULATEL_TOKEN": "", # replace with the token returned from the admin CLI
  },
  labels=["formulatel"]
)

docker_build("formulatel/persist", context="./formulatel", dockerfile="formulatel/persist.Dockerfile")
docker_build("formulatel/timescaledb-migrations", context=".", dockerfile="migrations.Dockerfile")

# our helm chart expects a few secrets to be available with DB login info
k8s_yaml("kubernetes/secrets.yaml")
k8s_resource(objects=["formulatel-db-user-persist"], new_name="db-user-persist-secret", labels=["infra"])

# finally, compile and deploy the formulatel chart
# this deploys grafana, mqtt, persist, and the telemetry schema migrations
k8s_yaml(helm("./kubernetes/charts/formulatel",
  name="formulatel",
  namespace="formulatel",
  values="./kubernetes/config/local/formulatel-values.yaml",
))
k8s_resource("formulatel-grafana", port_forwards=3000, labels=["infra"])
k8s_resource("formulatel-mosquitto", port_forwards=9001, labels=["infra"])
k8s_resource("formulatel-formulatel-persist", labels=["formulatel"])
k8s_resource("formulatel-formulatel-dbmigrate-latest", labels=["formulatel"])