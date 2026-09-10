output "server-public-ip" {
  value = oci_core_public_ip.static_ip.ip_address
}