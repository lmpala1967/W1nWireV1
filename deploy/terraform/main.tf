# =============================================================================
# W1nWire — Oracle Cloud Infrastructure (OCI) Always Free deployment.
#
# What this stack creates:
#   * a VCN (10.0.0.0/16) with a public subnet, an internet gateway,
#     a default route table and a security list,
#   * a compute instance (Always Free shape) with a reserved public IP,
#   * cloud-init user-data that installs Go, builds w1nwire-server and wires
#     up systemd, sysctl, NAT and the firewall.
#
# -----------------------------------------------------------------------------
# IMPORTANT NOTES FOR THE ALWAYS FREE TIER (2026)
# -----------------------------------------------------------------------------
# 1. SHAPE CAPACITY. The Ampere shapes (`VM.Standard.A1.Flex`) are almost
#    always "Out of host capacity" in the popular regions. When `apply` fails,
#    change `region` or switch to `instance_shape = "VM.Standard.E2.1.Micro"`
#    (two free micro instances per tenancy, and they are almost always
#    available).
# 2. FREE TIER vs PAID ACCOUNT. A brand new OCI tenancy starts as "Free Tier"
#    (no credit card). Older accounts and accounts that want to keep the
#    Always Free entitlements have to be on the "Pay As You Go" (PAYG) upgrade
#    path — the Always Free resources are still free, but the account is able
#    to create paid resources as well. Without the PAYG upgrade some regions
#    refuse to create Always Free shapes at all.
# 3. IDLE RECLAMATION. OCI may reclaim an Always Free compute instance that
#    stays under the CPU (< 10%), network (< 10%) and memory (< 10%)
#    thresholds for 7 consecutive days. `deploy/scripts/oracle-idle-keepalive.sh`
#    (installed by the bootstrap) prevents this.
# 4. ARM IMAGES. The Ubuntu image is looked up with the `shape` filter of the
#    `oci_core_images` data source so that an aarch64 image is picked for the
#    ARM shape and an x86_64 image for the AMD shape.
# =============================================================================

# -----------------------------------------------------------------------------
# Provider
# -----------------------------------------------------------------------------

provider "oci" {
  region         = var.region
  tenancy        = var.tenancy
  compartment_id = var.compartment_ocid

  # The OCI API is aggressively throttled; retry a few times before failing.
  retry_policy {
    max_retries     = 5
    retry_backoff   = 10
    http_debug_mode = "INFO"
  }
}

# -----------------------------------------------------------------------------
# Locals
# -----------------------------------------------------------------------------

locals {
  # Short random suffix so that DNS labels do not collide with an already
  # existing VCN/subnet of the same name in the region.
  name_suffix = random_string.suffix.result

  # Flex shapes accept a `shape_config` block, fixed shapes (E2.1.Micro, ...)
  # reject it with "shape_config is not supported for this shape".
  is_flex_shape = can(regex("Flex$", var.instance_shape))

  # Tags applied to every taggable resource.
  common_tags = merge(
    {
      project     = var.project_name
      managed_by  = "terraform"
      repository  = "W1nWire"
      environment = "always-free"
    },
    var.freeform_tags,
  )

  # Addresses inside the tunnel. The server takes the first host of the
  # network, the first client the second one - the same convention the
  # `w1nwire-client gen-config` command uses.
  tunnel_prefix       = split("/", var.tunnel_cidr)[1]
  tunnel_server_ip    = cidrhost(var.tunnel_cidr, 1)
  tunnel_server_cidr  = "${cidrhost(var.tunnel_cidr, 1)}/${split("/", var.tunnel_cidr)[1]}"
  tunnel_client_ip    = cidrhost(var.tunnel_cidr, 2)
  tunnel_client_cidr  = "${cidrhost(var.tunnel_cidr, 2)}/32"

  # Rendered cloud-init payload. `templatefile()` renders the shell script,
  # `base64encode()` is required because OCI expects the user-data to be
  # base64 encoded inside the instance metadata.
  user_data = base64encode(templatefile(
    "${path.module}/../cloud-init/user-data.sh",
    {
      go_version        = var.go_version
      go_sha256_amd64   = var.go_sha256_amd64
      go_sha256_arm64   = var.go_sha256_arm64
      repo_url          = var.repo_url
      repo_branch       = var.repo_branch
      vpn_port          = var.vpn_port
      tunnel_cidr       = var.tunnel_cidr
      tunnel_server_cidr = local.tunnel_server_cidr
      tunnel_server_ip  = local.tunnel_server_ip
      tunnel_client_cidr = local.tunnel_client_cidr
      tunnel_client_ip  = local.tunnel_client_ip
      tun_interface     = var.tun_interface
      tun_mtu           = var.tun_mtu
      allowed_ssh_cidr  = var.allowed_ssh_cidr
      server_cmd_path   = var.server_cmd_path
      client_cmd_path   = var.client_cmd_path
      install_keepalive = var.install_keepalive
      server_unit       = file("${path.module}/../systemd/w1nwire-server.service")
      client_unit       = file("${path.module}/../systemd/w1nwire-client.service")
      keepalive_script  = file("${path.module}/../scripts/oracle-idle-keepalive.sh")
    },
  ))
}

# -----------------------------------------------------------------------------
# Random suffix for the DNS labels
# -----------------------------------------------------------------------------

resource "random_string" "suffix" {
  length  = 4
  lower   = true
  upper   = false
  numeric = true
  special = false
}

# -----------------------------------------------------------------------------
# Discovery
# -----------------------------------------------------------------------------

# The first availability domain of the region. Any AD works for a single
# instance; the first one is simply the most convenient.
data "oci_identity_availability_domains" "ad" {
  compartment_id = var.tenancy
}

# Ubuntu 22.04 LTS. The `shape` filter makes the provider return an image
# that is compatible with the architecture of the selected shape
# (aarch64 for A1.Flex, x86_64 for E2.1.Micro).
data "oci_core_images" "ubuntu_2204" {
  compartment_id           = var.compartment_ocid
  operating_system         = "Canonical Ubuntu"
  operating_system_version = "22.04"
  shape                    = var.instance_shape
  state                    = "AVAILABLE"
  sort_by                  = "TIMECREATED"
  sort_order               = "DESC"
}

# Existence / availability check of the requested shape. A missing shape
# produces a readable error instead of an opaque API failure.
data "oci_core_shapes" "w1nwire" {
  compartment_id = var.compartment_ocid
  shape          = var.instance_shape
}

# -----------------------------------------------------------------------------
# Networking
# -----------------------------------------------------------------------------

resource "oci_core_vcn" "w1nwire" {
  compartment_id = var.compartment_ocid
  cidr_block     = "10.0.0.0/16"
  dns_label      = "${var.project_name}${local.name_suffix}"
  display_name   = "${var.project_name}-vcn"
  freeform_tags  = local.common_tags
}

resource "oci_core_internet_gateway" "w1nwire" {
  compartment_id = var.compartment_ocid
  vcn_id         = oci_core_vcn.w1nwire.id
  display_name   = "${var.project_name}-igw"
  enabled        = true
  freeform_tags  = local.common_tags
}

resource "oci_core_route_table" "w1nwire" {
  compartment_id = var.compartment_ocid
  vcn_id         = oci_core_vcn.w1nwire.id
  display_name   = "${var.project_name}-route-table"

  route_rules {
    destination       = "0.0.0.0/0"
    destination_type  = "CIDR_BLOCK"
    network_entity_id = oci_core_internet_gateway.w1nwire.id
  }

  freeform_tags = local.common_tags
}

resource "oci_core_vcn_dhcp_options" "w1nwire" {
  compartment_id = var.compartment_ocid
  vcn_id         = oci_core_vcn.w1nwire.id
  display_name   = "${var.project_name}-dhcp-options"

  # Public resolvers — the VPN server forwards DNS for the tunnel clients.
  options {
    type           = "DomainNameServer"
    server         = ["1.1.1.1", "1.0.0.1"]
    search_domains = ["w1nwire.internal"]
  }

  freeform_tags = local.common_tags
}

# NOTE: the VCN uses 10.0.0.0/16, therefore 10.0.0.0/24 is reserved for the
# regional subnet; the public subnet uses the second /24.
resource "oci_core_subnet" "w1nwire" {
  compartment_id  = var.compartment_ocid
  vcn_id          = oci_core_vcn.w1nwire.id
  cidr_block      = "10.0.1.0/24"
  dns_label       = "${var.project_name}pub${local.name_suffix}"
  display_name    = "${var.project_name}-public-subnet"
  route_table_id  = oci_core_route_table.w1nwire.id
  dhcp_options_id = oci_core_vcn_dhcp_options.w1nwire.id

  security_list_ids = [oci_core_security_list.w1nwire.id]

  # Public subnet: instances may hold a public IP address.
  prohibit_public_ip_on_vnic = false

  freeform_tags = local.common_tags
}

resource "oci_core_security_list" "w1nwire" {
  compartment_id = var.compartment_ocid
  vcn_id         = oci_core_vcn.w1nwire.id
  display_name   = "${var.project_name}-security-list"

  # Ingress: SSH. `allowed_ssh_cidr` defaults to 0.0.0.0/0 — TODO: restrict it
  # to your own public IP before exposing the instance to the internet.
  ingress_security_rules {
    stateless = false
    source    = var.allowed_ssh_cidr
    protocol  = "6" # TCP
    tcp_options {
      min = 22
      max = 22
    }
  }

  # Ingress: the W1nWire protocol (custom UDP protocol).
  ingress_security_rules {
    stateless = false
    source    = "0.0.0.0/0"
    protocol  = "17" # UDP
    udp_options {
      min = var.vpn_port
      max = var.vpn_port
    }
  }

  # Egress: everything (the server needs internet access for package
  # installation, Go downloads and the idle keepalive traffic).
  egress_security_rules {
    stateless   = false
    destination = "0.0.0.0/0"
    protocol    = "all"
  }

  freeform_tags = local.common_tags
}

# -----------------------------------------------------------------------------
# Compute
# -----------------------------------------------------------------------------

resource "oci_core_instance" "w1nwire" {
  compartment_id      = var.compartment_ocid
  availability_domain = data.oci_identity_availability_domains.ad.availability_domains[0].name
  display_name        = "${var.project_name}-server"
  shape               = var.instance_shape

  # `shape_config` is only accepted by Flex shapes.
  dynamic "shape_config" {
    for_each = local.is_flex_shape ? [1] : []
    content {
      ocpus         = var.instance_ocpus
      memory_in_gbs = var.instance_memory_gb
    }
  }

  source_details {
    source_type             = "image"
    source_id               = data.oci_core_images.ubuntu_2204.images[0].id
    boot_volume_size_in_gbs = var.boot_volume_size_gbs
  }

  create_vnic_details {
    subnet_id        = oci_core_subnet.w1nwire.id
    display_name     = "${var.project_name}-primary-vnic"
    assign_public_ip = false # a RESERVED public IP is attached below
    hostname_label   = "${var.project_name}${local.name_suffix}"
  }

  metadata = {
    ssh_authorized_keys = var.ssh_public_key
    user_data           = local.user_data
  }

  freeform_tags = local.common_tags

  lifecycle {
    precondition {
      condition     = length(data.oci_core_shapes.w1nwire.shapes) > 0
      error_message = "The shape '${var.instance_shape}' does not exist in region '${var.region}'. Use a shape that is offered in this region (for example VM.Standard.A1.Flex or VM.Standard.E2.1.Micro)."
    }

    precondition {
      condition     = length(data.oci_core_images.ubuntu_2204.images) > 0
      error_message = "No Canonical Ubuntu 22.04 image found for shape '${var.instance_shape}' in region '${var.region}'."
    }
  }
}

# -----------------------------------------------------------------------------
# Reserved public IP
#
# The instance is created without an ephemeral public IP and a RESERVED public
# IP is attached to its primary private IP afterwards. Reserved addresses keep
# the VPN endpoint stable across stop/start cycles, which matters because the
# endpoint is baked into every client configuration.
# -----------------------------------------------------------------------------

data "oci_core_vnic_attachments" "w1nwire" {
  compartment_id = var.compartment_ocid
  instance_id    = oci_core_instance.w1nwire.id

  depends_on = [oci_core_instance.w1nwire]
}

data "oci_core_private_ips" "w1nwire_primary" {
  vnic_id = data.oci_core_vnic_attachments.w1nwire.vnic_attachments[0].vnic_id

  depends_on = [oci_core_instance.w1nwire]
}

resource "oci_core_public_ip" "w1nwire" {
  compartment_id = var.compartment_ocid
  display_name   = "${var.project_name}-public-ip"
  lifetime       = "RESERVED"

  # Primary private IP of the instance VNIC. The lookup is defensive: if the
  # primary IP cannot be determined the reserved address is still created
  # (unassigned) and can be attached from the OCI console.
  private_ip_id = try(
    one([for ip in data.oci_core_private_ips.w1nwire_primary.private_ips : ip.id if ip.is_primary]),
    null,
  )

  freeform_tags = local.common_tags

  depends_on = [oci_core_instance.w1nwire]
}
