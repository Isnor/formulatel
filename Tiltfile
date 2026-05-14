load('ext://helm_resource', 'helm_resource', 'helm_repo')
# this repo gives us an easy way to start an MQTT broker, mosquitto
helm_repo("k8s-at-home", resource_name="k8s-at-home_helm_repo", url="https://k8s-at-home.com/charts/", labels=["helm"])
helm_repo("grafana", resource_name="grafana_helm_repo", url="https://grafana.github.io/helm-charts", labels=["helm"])

# TODO: probably remove this and use `tilt up --namespace=`. namespace will need to be created before tilt is run
k8s_yaml("kubernetes/namespace.yml")

helm_resource("grafana", chart="grafana/grafana", namespace="formulatel", flags=["--values", "./kubernetes/config/grafana-values.yml", "--set-file", "dashboards.formulatel.live.json=./kubernetes/config/live_dash_v2.json"], port_forwards="3000", labels=["infra"])
helm_resource("mosquitto", chart="k8s-at-home/mosquitto", namespace="formulatel", port_forwards="1883", labels=["infra"])

# set up a postgres instance with timescaleDB
k8s_yaml("kubernetes/datastore.yml")
k8s_resource("timescaledb", port_forwards="5432", labels=["infra"])

# build, run, and reload ingest
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

# build the persist image used for the persist kubernetes service
docker_build("persist", context="./formulatel", dockerfile="formulatel/persist.Dockerfile")
k8s_yaml("kubernetes/persist.yml")
k8s_resource("persist", labels=["formulatel"])