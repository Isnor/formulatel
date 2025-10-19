load('ext://helm_resource', 'helm_resource', 'helm_repo')
# this repo gives us an easy way to start an MQTT broker, mosquitto
helm_repo("k8s-at-home", resource_name="k8s-at-home_helm_repo", url="https://k8s-at-home.com/charts/", labels=["helm"])
helm_repo("grafana", resource_name="grafana_helm_repo", url="https://grafana.github.io/helm-charts", labels=["helm"])

# TODO: probably remove this and use `tilt up --namespace=`. namespace will need to be created before tilt is run
k8s_yaml("kubernetes/namespace.yml")

helm_resource("grafana", chart="grafana/grafana", namespace="formulatel", flags=["--values", "./kubernetes/config/grafana-values.yml"], port_forwards="3000", labels=["infra"])
helm_resource("mosquitto", chart="k8s-at-home/mosquitto", namespace="formulatel", port_forwards="1883", labels=["infra"])

# local_resource(
#   "formulatel_ingest",
#   cmd="make build",
#   serve_cmd="./out/ingest",
#   # trigger_mode=TRIGGER_MODE_MANUAL,
#   deps=[
#     "./formulatel/cmd/ingest",
#     "./formulatel/f123",
#   ],
#   labels=["formulatel"]
# )

# local_resource(
#   "formulatel_gentel",
#   cmd="make build",
#   serve_cmd="./out/gentel -frequency=60 -max-gear=10 -max-rpm=15000 -max-kph=400 -acceleration=300000",
#   # trigger_mode=TRIGGER_MODE_MANUAL,
#   deps=[
#     "./formulatel/cmd/gentel",
#     "./formulatel/f123",
#   ],
#   labels=["formulatel"]
# )

# docker_build("formulatel_persist", ".", dockerfile="Dockerfile", only=[
#   "./formulatel/",
#   "./protobuf/",
#   "./Makefile",
# ], labels=["formulatel"])
# k8s_yaml("kubernetes/persist.yml", labels=["formulatel"])
# k8s_resource("persist", labels=["formulatel"])