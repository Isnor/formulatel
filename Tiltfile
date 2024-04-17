load('ext://helm_resource', 'helm_resource', 'helm_repo')
k8s_yaml("kubernetes/namespace.yml")


helm_resource("otel-col", chart="open-telemetry/opentelemetry-collector", namespace="formulatel", flags=["--values", "./kubernetes/config/collector-values.yml"])
helm_resource("kafka", chart="oci://registry-1.docker.io/bitnamicharts/kafka", namespace="formulatel")
helm_resource("opensearch", chart="opensearch/opensearch", namespace="formulatel", flags=["--values", "./kubernetes/config/opensearch-values.yml"])
helm_resource("opensearch-dashboards", chart="opensearch/opensearch-dashboards", namespace="formulatel", port_forwards="5601")

# formulatel-rpc is the gRPC server that accepts data from the ingestion service and sends it to the 
# processing pipeline
docker_build("formulatel_rpc", ".", dockerfile="Dockerfile", only=[
  "./formulatel/",
  "./protobuf/",
  "./Makefile",
])

k8s_yaml("kubernetes/rpc.yml")
k8s_resource("rpc", port_forwards="29292")
# k8s_resource("opensearch-dashboards", port_forwards="8080")