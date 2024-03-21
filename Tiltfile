# namespace = "formulatel"

k8s_yaml("kubernetes/namespace.yml")
# k8s_yaml("kubernetes/grafana.yml")
k8s_yaml("kubernetes/otel-collector.yml")
# k8s_yaml("kubernetes/prometheus.yml")
k8s_yaml("kubernetes/prom-grafana.yml")
k8s_yaml("kubernetes/crds.yml")

# k8s_kind("Alertmanager")
k8s_resource("formulatel-grafana", port_forwards=3000, )

# k8s_resource("formulatel-opentelemetry-collector")
# k8s_resource("formulatel-prometheus")

# docker_build("formulatelbase", ".", dockerfile="base.Dockerfile")
# # the ingest service is responsible for ingesting telemetry from various sources, such as F123,
# # converting it to our telemetry protobufs (T->proto(T)), and sending it to the formulatel service
# docker_build("formulatel_ingest", ".", dockerfile="ingest.Dockerfile")


# helm("open-telemetry/opentelemetry-collector")
# helm("open-telemetry/opentelemetry-operator")


# k8s_yaml("kubernetes/ingest.yml")
# k8s_resource("ingest")

# formulatel is the gRPC server that accepts data from the ingestion service and sends it to the 
# processing pipeline
# docker_build(f"${namespace}-rpc")