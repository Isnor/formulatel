k8s_yaml("kubernetes/namespace.yml")

# k8s_resource("formulatel-grafana", port_forwards=3000)
# k8s_resource("formulatel-prometheus-server", port_forwards=9090)

k8s_yaml("kubernetes/anothercollector.yml")
k8s_resource("formulatel-grafana", port_forwards=3000)
k8s_resource("formulatel-prometheus-server", port_forwards=9090)
# formulatel-rpc is the gRPC server that accepts data from the ingestion service and sends it to the 
# processing pipeline
docker_build("formulatel_rpc", ".", dockerfile="Dockerfile", only=[
  "./formulatel/",
  "./protobuf/",
  "./Makefile",
])

k8s_yaml("kubernetes/rpc.yml")
k8s_resource("rpc", port_forwards="29292")
