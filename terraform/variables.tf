variable "compartment_ocid" {
  type = string
  description = "ID of the compartment to deploy these resources in"
}

variable "availability_domain" {
  type = string
  description = "based on the region being deployed into. `oci iam availability-domain list` will list ADs"
}

variable "home_ip" {
  type = string
  description = "an IPv4 address to allow SSH traffic from"
}

variable "ubuntu_arm_image_ocid" {
  type = string
  description = "ID of the ubuntu image to use"
  # ID of the VM image for Ubuntu `24.04-Minimal-aarch64-2026.04.30-1`, the latest minimal Ubuntu image
  # available for ARM64 at the time of writing
  default = "ocid1.image.oc1.ca-montreal-1.aaaaaaaaavxe7ctjwec2f6pbyuj6vfxuncmeh4dv57sx6r2mqbuvf7zklmya"
}