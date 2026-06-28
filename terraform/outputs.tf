output "server-public-ip" {
  value = oci_core_instance.single_node_k8s.public_ip
}