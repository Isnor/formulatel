load('ext://helm_resource', 'helm_resource', 'helm_repo')
# this repo gives us an easy way to start an MQTT broker, mosquitto 
helm_repo("k8s-at-home_helm_repo", url="https://k8s-at-home.com/charts/")
# helm_repo("open-telemetry_helm_repo", url="https://open-telemetry.github.io/opentelemetry-helm-charts")
# helm_repo("prometheus-community_helm_repo", url="https://prometheus-community.github.io/helm-charts")
# helm_repo("opentelemetry_helm_repo", url="https://open-telemetry.github.io/opentelemetry-helm-charts")
# helm_repo("bitnami_helm_repo", url="https://charts.bitnami.com/bitnami")
# helm_repo("opensearch_helm_repo", url="https://opensearch-project.github.io/helm-charts/")
helm_repo("grafana_helm_repo", url="https://grafana.github.io/helm-charts")

# TODO: probably remove this and use `tilt up --namespace=`. namespace will need to be created before tilt is run
k8s_yaml("kubernetes/namespace.yml")

# helm_resource("otel-col", chart="open-telemetry/opentelemetry-collector", namespace="formulatel", flags=["--values", "./kubernetes/config/collector-values.yml"])
# helm_resource("kafka", chart="oci://registry-1.docker.io/bitnamicharts/kafka", namespace="formulatel",  flags=["--values", "./kubernetes/config/kafka-values.yml"])
# helm_resource("opensearch", chart="opensearch/opensearch", namespace="formulatel", flags=["--values", "./kubernetes/config/opensearch-values.yml"], port_forwards="9200")
# helm_resource("opensearch-dashboards", chart="opensearch/opensearch-dashboards", namespace="formulatel", flags=["--values", "./kubernetes/config/opensearch-dashboards-values.yml"], port_forwards="5601")
helm_resource("grafana", chart="grafana/grafana", namespace="formulatel", flags=["--values", "./kubernetes/config/grafana-values.yml"], port_forwards="3000")
# helm_resource("prometheus", chart="prometheus-community/kube-prometheus-stack", namespace="formulatel", port_forwards="3000")
helm_resource("mosquitto", chart="k8s-at-home/mosquitto", namespace="formulatel", port_forwards="1883")
# docker_build("formulatel_persist", ".", dockerfile="Dockerfile", only=[
#   "./formulatel/",
#   "./protobuf/",
#   "./Makefile",
# ])

# k8s_yaml("kubernetes/persist.yml")
# k8s_resource("persist")