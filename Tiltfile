load('ext://helm_resource', 'helm_resource', 'helm_repo')
k8s_yaml("kubernetes/namespace.yml")


helm_resource("otel-col", chart="open-telemetry/opentelemetry-collector", namespace="formulatel", flags=["--values", "./kubernetes/config/collector-values.yml"])
helm_resource("kafka", chart="oci://registry-1.docker.io/bitnamicharts/kafka", namespace="formulatel",  flags=["--values", "./kubernetes/config/kafka-values.yml"], port_forwards="9092")
helm_resource("opensearch", chart="opensearch/opensearch", namespace="formulatel", flags=["--values", "./kubernetes/config/opensearch-values.yml"], port_forwards="9200")
helm_resource("opensearch-dashboards", chart="opensearch/opensearch-dashboards", namespace="formulatel", port_forwards="5601")


docker_build("formulatel_persist", ".", dockerfile="Dockerfile", only=[
  "./formulatel/",
  "./protobuf/",
  "./Makefile",
])

k8s_yaml("kubernetes/persist.yml")
k8s_resource("persist")