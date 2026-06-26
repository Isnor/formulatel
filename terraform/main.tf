terraform {
  required_providers {
    oci = {
      source  = "oracle/oci"
      version = "8.20.0"
    }
  }
}

provider "oci" {}