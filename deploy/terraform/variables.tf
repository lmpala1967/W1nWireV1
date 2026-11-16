# =============================================================================
# W1nWire — input variables for the OCI Always Free deployment.
#
# Nothing secret belongs in this repository. `ssh_public_key` is a *public* key
# (safe to commit), but OCIDs and any private material stay in `*.tfvars`
# which is git-ignored.
# =============================================================================

# -----------------------------------------------------------------------------
# OCI tenancy / compartment
# -----------------------------------------------------------------------------

variable "region" {
  description = <<-EOT
    OCI region identifier.

    Default is `eu-frankfurt-1` (cheap egress to Europe, adequate latency).

    NOTE ON ARM CAPACITY: the Always Free Ampere shapes (`VM.Standard.A1.Flex`)
    are frequently OUT OF CAPACITY in popular regions — most notably in
    `eu-frankfurt-1`, `us-ashburn-1` and `us-phoenix-1`. When `terraform apply`
    fails with "Out of host capacity", switch `region` to another one
    (for example `eu-zurich-1`, `eu-amsterdam-1`, `uk-london-1`, `ap-mumbai-1`,
    `ap-tokyo-1`, `sa-saopaulo-1`) or fall back to the AMD micro shape
    (`instance_shape = "VM.Standard.E2.1.Micro"`), which is much easier to
    provision.
  EOT
  type        = string
  default     = "eu-frankfurt-1"

  validation {
    condition     = can(regex("^[a-z]{2}-[a-z]+-[0-9]+$", var.region))
    error_message = "region must look like 'eu-frankfurt-1'."
  }
}

variable "compartment_ocid" {
  description = <<-EOT
    OCID of the compartment that will hold the W1nWire resources.

    Use the tenancy root compartment OCID for the simplest setup; the tenancy
    OCID is also a valid compartment OCID.
  EOT
  type        = string

  validation {
    condition     = can(regex("^ocid1\\.compartment\\.", var.compartment_ocid))
    error_message = "compartment_ocid must be an OCI compartment OCID."
  }
}

variable "tenancy" {
  description = <<-EOT
    OCID of the OCI tenancy (the top level "root" tenancy account).

    Required by the OCI provider for API calls that are not compartment scoped.
  EOT
  type        = string

  validation {
    condition     = can(regex("^ocid1\\.tenancy\\.", var.tenancy))
    error_message = "tenancy must be an OCI tenancy OCID."
  }
}

# -----------------------------------------------------------------------------
# Naming
# -----------------------------------------------------------------------------

variable "project_name" {
  description = "Prefix used for every resource name (display names, DNS labels, tags)."
  type        = string
  default     = "w1nwire"

  validation {
    condition     = can(regex("^[a-z][a-z0-9-]{1,20}$", var.project_name))
    error_message = "project_name must be lowercase alphanumeric/dash, 2-21 chars."
  }
}

# -----------------------------------------------------------------------------
# Access
# -----------------------------------------------------------------------------

variable "ssh_public_key" {
  description = <<-EOT
    Public SSH key (OpenSSH format) that is installed into the instance via
    cloud-init `ssh_authorized_keys`.

    This is a PUBLIC key — it is safe to keep in the repository, but it is
    still declared as a variable so that a personal key can be supplied via
    `terraform.tfvars` instead.
  EOT
  type        = string
  default     = ""

  validation {
    condition     = var.ssh_public_key == "" || can(regex("^(ssh-rsa|ssh-ed25519|ecdsa-sha2-nistp[0-9]+) ", var.ssh_public_key))
    error_message = "ssh_public_key must be an OpenSSH public key (ssh-ed25519, ssh-rsa or ecdsa-*)."
  }
}

variable "allowed_ssh_cidr" {
  description = <<-EOT
    CIDR block that is allowed to reach the SSH port (22/tcp).

    TODO: the default `0.0.0.0/0` exposes the instance to the whole internet
    and will be picked up by port scanners within minutes. Before any real
    deployment, replace it with your own public IP, e.g. `203.0.113.7/32`.
    A convenient way to obtain it: `curl -s https://api.ipify.org`.
  EOT
  type        = string
  default     = "0.0.0.0/0"

  validation {
    condition     = can(cidrnetmask(var.allowed_ssh_cidr))
    error_message = "allowed_ssh_cidr must be a valid CIDR block (e.g. 203.0.113.7/32)."
  }
}

# -----------------------------------------------------------------------------
# Compute
# -----------------------------------------------------------------------------

variable "instance_shape" {
  description = <<-EOT
    Shape used for the W1nWire server instance.

    Always Free options (per tenancy, as of the 2026 free tier):

      * `VM.Standard.A1.Flex`  — Ampere ARM, 4 OCPU / 24 GB per instance.
        Best performance, but capacity is exhausted in most regions.
        Configure it with `instance_ocpus` / `instance_memory_gb`.
        Free allowance: 4 OCPU / 24 GB total across ARM instances.

      * `VM.Standard.E2.1.Micro` — AMD micro (1/8 OCPU, 1 GB RAM).
        Two instances per tenancy are free and are usually available.
        `instance_ocpus` / `instance_memory_gb` are ignored for this shape
        (it is not a Flex shape), so the box is small but sufficient for a
        demo VPN server.

    Any other shape works as well but is billed against the (zero) credit of
    the account — the Always Free allowance only covers the shapes above.
  EOT
  type        = string
  default     = "VM.Standard.A1.Flex"

  validation {
    condition     = can(regex("^VM\\.Standard\\.[A-Za-z0-9]+\\.[A-Za-z0-9.]+$", var.instance_shape))
    error_message = "instance_shape must be a valid OCI shape name, e.g. VM.Standard.A1.Flex."
  }
}

variable "instance_ocpus" {
  description = <<-EOT
    Number of OCPUs for Flex shapes.

    Ignored for non-Flex shapes (e.g. `VM.Standard.E2.1.Micro`), where OCI
    rejects the `shape_config` block entirely.
  EOT
  type        = number
  default     = 2

  validation {
    condition     = var.instance_ocpus >= 1 && var.instance_ocpus <= 4
    error_message = "instance_ocpus must be between 1 and 4 (Always Free ARM limit)."
  }
}

variable "instance_memory_gb" {
  description = <<-EOT
    RAM in GB for Flex shapes.

    Ignored for non-Flex shapes. The Always Free ARM allowance is 24 GB per
    instance and 9000 GB-hours per month in total.
  EOT
  type        = number
  default     = 12

  validation {
    condition     = var.instance_memory_gb >= 1 && var.instance_memory_gb <= 24
    error_message = "instance_memory_gb must be between 1 and 24 (Always Free ARM limit)."
  }
}

variable "boot_volume_size_gbs" {
  description = <<-EOT
    Size of the boot volume in GB.

    Always Free includes 200 GB of block volume per tenancy, so 50 GB (the
    minimum for an OCI boot volume) is the sensible choice.
  EOT
  type        = number
  default     = 50

  validation {
    condition     = var.boot_volume_size_gbs >= 50 && var.boot_volume_size_gbs <= 200
    error_message = "boot_volume_size_gbs must be between 50 and 200 (Always Free block volume allowance)."
  }
}

# -----------------------------------------------------------------------------
# W1nWire application
# -----------------------------------------------------------------------------

variable "vpn_port" {
  description = <<-EOT
    UDP port the W1nWire protocol listens on.

    The protocol is a custom UDP protocol, so any port works; 51820 is used
    as it is the de-facto default for UDP based VPNs.
  EOT
  type        = number
  default     = 51820

  validation {
    condition     = var.vpn_port > 1024 && var.vpn_port < 65536
    error_message = "vpn_port must be a privileged-free UDP port between 1025 and 65535."
  }
}

variable "tunnel_cidr" {
  description = <<-EOT
    IPv4 CIDR used inside the tunnel (the "office" network behind the VPN).

    The same value must be used by the server and by every client, and it must
    not overlap with any local network of the client.
  EOT
  type        = string
  default     = "10.66.0.0/24"

  validation {
    condition     = can(cidrnetmask(var.tunnel_cidr))
    error_message = "tunnel_cidr must be a valid IPv4 CIDR block."
  }
}

variable "tun_interface" {
  description = <<-EOT
    Name of the TUN device created by the W1nWire server (e.g. w1n0).

    The name has to be unique on the host and short enough for the Linux
    interface name limit (IFNAMSIZ = 16 characters).
  EOT
  type        = string
  default     = "w1n0"

  validation {
    condition     = can(regex("^[a-z0-9]{2,15}$", var.tun_interface))
    error_message = "tun_interface must be a short lowercase interface name."
  }
}

variable "tun_mtu" {
  description = <<-EOT
    MTU of the tunnel interface.

    1420 is the value the `w1nwire-client gen-config` command uses and it
    fits into the 1500 byte Ethernet MTU without fragmentation.
  EOT
  type        = number
  default     = 1420

  validation {
    condition     = var.tun_mtu >= 1280 && var.tun_mtu <= 1500
    error_message = "tun_mtu must be between 1280 and 1500."
  }
}

variable "repo_url" {
  description = <<-EOT
    Git URL of the W1nWire repository that is cloned on the instance.

    Leave empty to skip the clone — the bootstrap then falls back to the
    pre-uploaded tarball `/opt/w1nwire/w1nwire-src.tar.gz` and, if that is
    missing too, installs a service that fails with an explanatory message.
  EOT
  type        = string
  default     = ""
}

variable "repo_branch" {
  description = "Branch that is checked out on the instance."
  type        = string
  default     = "main"
}

variable "server_cmd_path" {
  description = <<-EOT
    Go package path of the server command inside the repository.

    Used to build the binary with `go build`. If the path does not exist the
    bootstrap script tries to auto-detect a `cmd/*server*` package.
  EOT
  type        = string
  default     = "./cmd/w1nwire-server"
}

variable "client_cmd_path" {
  description = "Go package path of the client command inside the repository."
  type        = string
  default     = "./cmd/w1nwire-client"
}

variable "go_version" {
  description = <<-EOT
    Go toolchain version installed on the instance.

    The bootstrap downloads the official tarball from go.dev and verifies its
    SHA-256 checksum (both amd64 and arm64 checksums are pinned in
    `variables.tf`). The value must satisfy the `go` directive of the
    repository `go.mod`; newer toolchains are downloaded automatically by Go
    itself (GOTOOLCHAIN=auto).
  EOT
  type        = string
  default     = "1.24.0"

  validation {
    condition     = can(regex("^[0-9]+\\.[0-9]+(\\.[0-9]+)?$", var.go_version))
    error_message = "go_version must look like 1.24.0."
  }
}

# -----------------------------------------------------------------------------
# Misc
# -----------------------------------------------------------------------------

variable "go_sha256_amd64" {
  description = <<-EOT
    SHA-256 checksum of the official Go tarball for linux/amd64.

    Pinned for `go_version = 1.24.0`. When `go_version` is bumped, the
    matching checksum has to be updated as well — the bootstrap refuses to
    install an unverified toolchain.
  EOT
  type        = string
  default     = "dea9ca38a0b852a74e81c26134671af7c0fbe65d81b0dc1c5bfe22cf7d4c8858"
}

variable "go_sha256_arm64" {
  description = <<-EOT
    SHA-256 checksum of the official Go tarball for linux/arm64.

    Pinned for `go_version = 1.24.0`. See `go_sha256_amd64`.
  EOT
  type        = string
  default     = "c3fa6d16ffa261091a5617145553c71d21435ce547e44cc6dfb7470865527cc7"
}

variable "ssh_user" {
  description = "Login user of the Ubuntu image (also used in the SSH hint output)."
  type        = string
  default     = "ubuntu"
}

variable "install_keepalive" {
  description = <<-EOT
    Install the idle-keepalive cron job.

    OCI may reclaim ("idle reclamation") an Always Free compute instance when
    it stays below the CPU / network / memory thresholds for 7 days. The
    keepalive script generates a small amount of CPU and network activity so
    that the instance is never classified as idle.
  EOT
  type        = bool
  default     = true
}

variable "freeform_tags" {
  description = "Extra free-form tags applied to every taggable resource."
  type        = map(string)
  default     = {}
}
