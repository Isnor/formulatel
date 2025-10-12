load('ext://helm_resource', 'helm_resource', 'helm_repo')
# this repo gives us an easy way to start an MQTT broker, mosquitto 
helm_repo("k8s-at-home_helm_repo", url="https://k8s-at-home.com/charts/", labels=["helm"])
helm_repo("grafana_helm_repo", url="https://grafana.github.io/helm-charts", labels=["helm"])

# TODO: probably remove this and use `tilt up --namespace=`. namespace will need to be created before tilt is run
k8s_yaml("kubernetes/namespace.yml")

helm_resource("grafana", chart="grafana/grafana", namespace="formulatel", flags=["--values", "./kubernetes/config/grafana-values.yml"], port_forwards="3000", labels=["infra"])
helm_resource("mosquitto", chart="k8s-at-home/mosquitto", namespace="formulatel", port_forwards="1883", labels=["infra"])

local_resource(
  "build_ingest",
  cmd="make build",
  # trigger_mode=TRIGGER_MODE_MANUAL,
  deps=[
    "./formulatel/cmd/ingest",
  ],
  labels=["formulatel"]
)

local_resource(
  "run_ingest",
  serve_cmd="./out/ingest",
  deps=[
    "./out/ingest",
  ],
  labels=["formulatel"],
)


# docker_build("formulatel_persist", ".", dockerfile="Dockerfile", only=[
#   "./formulatel/",
#   "./protobuf/",
#   "./Makefile",
# ], labels=["formulatel"])
# k8s_yaml("kubernetes/persist.yml", labels=["formulatel"])
# k8s_resource("persist", labels=["formulatel"])