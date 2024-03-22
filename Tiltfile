k8s_yaml("kubernetes/namespace.yml")
k8s_yaml("kubernetes/otel-collector.yml")
k8s_yaml("kubernetes/prom-grafana.yml")
k8s_yaml("kubernetes/crds.yml")

k8s_resource("formulatel-grafana", port_forwards=3000)

docker_build("formulatelbase", ".", dockerfile="base.Dockerfile")

# the ingest service is responsible for ingesting telemetry from various sources, such as F123,
# converting it to our telemetry protobufs (T->proto(T)), and sending it to the formulatel service
docker_build("formulatel_rpc", ".", dockerfile="rpc.Dockerfile", only=["forumlatel/"])


k8s_yaml("kubernetes/rpc.yml")
k8s_resource("rpc", port_forwards="29292")

# formulatel-rpc is the gRPC server that accepts data from the ingestion service and sends it to the 
# processing pipeline