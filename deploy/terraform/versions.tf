# =============================================================================
# W1nWire — Terraform / provider version pinning.
#
# The OCI provider is published under the `oracle/oci` namespace on the
# Terraform Registry (the `hashicorp/oci` entry is only a mirror).
#
# Minimum versions:
#   * terraform >= 1.5  — required for `lifecycle { precondition }` blocks.
#   * oracle/oci  >= 6  — 6.x is the current major line of the OCI provider.
#   * hashicorp/random >= 3.5 — used for a short, unique name suffix so that
#     DNS labels (VCN / subnet) do not collide between deployments.
# =============================================================================

terraform {
  required_version = ">= 1.5.0"

  required_providers {
    oci = {
      source  = "oracle/oci"
      version = ">= 6.0"
    }

    random = {
      source  = "hashicorp/random"
      version = ">= 3.5"
    }
  }
}
