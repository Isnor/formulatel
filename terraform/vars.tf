variable "compartment_ocid" {
  type = string
  description = "ID of the compartment to deploy these resources in"
}

variable "availability_domain" {
  type = string
}

variable "home_ip" {
  type = string
  description = "an IPv4 address to allow SSH traffic from"
}

variable "ubuntu_arm_image_ocid" {
  type = string
}