# Create the Virtual Cloud Network (VCN)
resource "oci_core_vcn" "formulatel_vcn" {
  cidr_block     = "10.0.0.0/16"
  compartment_id = var.compartment_ocid
  display_name   = "formulatel-network"
}

# Internet Gateway to allow public traffic in
resource "oci_core_internet_gateway" "ig" {
  compartment_id = var.compartment_ocid
  vcn_id         = oci_core_vcn.formulatel_vcn.id
  display_name   = "internet-gateway"
}

# Route Table to route internet traffic
resource "oci_core_route_table" "rt" {
  compartment_id = var.compartment_ocid
  vcn_id         = oci_core_vcn.formulatel_vcn.id
  route_rules {
    destination       = "0.0.0.0/0"
    destination_type  = "CIDR_BLOCK"
    network_entity_id = oci_core_internet_gateway.ig.id
  }
}

# Public Subnet
resource "oci_core_subnet" "public_subnet" {
  compartment_id    = var.compartment_ocid
  cidr_block        = "10.0.1.0/24"
  vcn_id            = oci_core_vcn.formulatel_vcn.id
  route_table_id    = oci_core_route_table.rt.id
  display_name      = "public-subnet"
}

# Network Security Group
resource "oci_core_network_security_group" "vm_nsg" {
  compartment_id = var.compartment_ocid
  vcn_id         = oci_core_vcn.formulatel_vcn.id
  display_name   = "formulatel-security-group"
}

# allow all UDP traffic on a specific port for a mesh network using tailscale
resource "oci_core_network_security_group_security_rule" "tailscale_rule" {
  network_security_group_id = oci_core_network_security_group.vm_nsg.id
  direction                 = "INGRESS"
  protocol                  = "17" # UDP
  source                    = "0.0.0.0/0"
  source_type               = "CIDR_BLOCK"
  udp_options {
    destination_port_range {
      min = 41641
      max = 41641
    }
  }
}

# Restrict kube-server (6443) to specific IP address
resource "oci_core_network_security_group_security_rule" "k8s_rule" {
  network_security_group_id = oci_core_network_security_group.vm_nsg.id
  direction                 = "INGRESS"
  protocol                  = "6" # TCP
  source                    = "${var.home_ip}/32"
  source_type               = "CIDR_BLOCK"
  tcp_options {
    destination_port_range {
      min = 6443
      max = 6443
    }
  }
}

# web ports for Grafana
resource "oci_core_network_security_group_security_rule" "grafana_http_rule" {
  network_security_group_id = oci_core_network_security_group.vm_nsg.id
  direction                 = "INGRESS"
  protocol                  = "6" # TCP
  source                    = "0.0.0.0/0"
  source_type               = "CIDR_BLOCK"
  tcp_options {
    destination_port_range {
      min = 80
      max = 80
    }
  }
}

resource "oci_core_network_security_group_security_rule" "grafana_https_rule" {
  network_security_group_id = oci_core_network_security_group.vm_nsg.id
  direction                 = "INGRESS"
  protocol                  = "6" # TCP
  source                    = "0.0.0.0/0"
  source_type               = "CIDR_BLOCK"
  tcp_options {
    destination_port_range {
      min = 443
      max = 443
    }
  }
}

# MQTT port
# TODO: restrict this to some list of CIDR blocks instead of the entire world / single IP
resource "oci_core_network_security_group_security_rule" "mqtt_rule" {
  network_security_group_id = oci_core_network_security_group.vm_nsg.id
  direction                 = "INGRESS"
  protocol                  = "6"
  source                    = "${var.home_ip}/32"
  source_type               = "CIDR_BLOCK"
  tcp_options {
    destination_port_range {
      min = 1883
      max = 1883
    }
  }
}

# Always-Free Ampere Compute Instance - this is where our k8s and postgres install will live
resource "oci_core_instance" "single_node_k8s" {
  availability_domain = var.availability_domain
  compartment_id      = var.compartment_ocid
  shape               = "VM.Standard.A1.Flex" # The Always Free ARM Shape

  # the free tier allows us enough CPU and memory credits to run 4 OCPUs and 24GB of memory 24/7.
  # hopefully this is sufficient to run postgres+timescaleDB along with a slim kubernetes distribution
  # on Ubuntu
  shape_config {
    ocpus         = 4
    memory_in_gbs = 24
  }

  create_vnic_details {
    subnet_id        = oci_core_subnet.public_subnet.id
    nsg_ids          = [oci_core_network_security_group.vm_nsg.id]
    assign_public_ip = true
  }

  source_details {
    source_type = "image"
    source_id   = var.ubuntu_arm_image_ocid
  }

  metadata = {
    ssh_authorized_keys = file("~/.ssh/id_rsa.pub")
    user_data           = base64encode(file("${path.module}/setup-server.sh"))
  }

  lifecycle {
    ignore_changes = [
      metadata, # remove or comment to destructively update to test a new setup script
    ]
  }
}