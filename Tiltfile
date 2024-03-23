k8s_yaml("kubernetes/namespace.yml")
k8s_yaml("kubernetes/otel-collector.yml")
# k8s_yaml("kubernetes/prom-grafana.yml")
# k8s_yaml("kubernetes/crds.yml")
k8s_yaml("kubernetes/prom.yml")
k8s_yaml("kubernetes/grafana.yml")

k8s_resource("formulatel-grafana", port_forwards=3000)
k8s_resource("formulatel-prometheus-server", port_forwards=9090)

# TODO: since we aren't building the ingest image anymore (for now; see https://github.com/kubernetes/kubernetes/issues/47862), we don't need
# to do this base image + two smaller ones now. 
docker_build("formulatelbase", ".", dockerfile="base.Dockerfile")
# formulatel-rpc is the gRPC server that accepts data from the ingestion service and sends it to the 
# processing pipeline
docker_build("formulatel_rpc", ".", dockerfile="rpc.Dockerfile")

k8s_yaml("kubernetes/rpc.yml")
k8s_resource("rpc", port_forwards="29292")
