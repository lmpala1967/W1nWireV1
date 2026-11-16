# =============================================================================
# W1nWire — deployment outputs.
#
# The outputs are meant to be consumed directly:
#
#   export TF_VAR_OUT=$(terraform output -json)
#   terraform output public_ip
#   terraform output ssh_command
#
# =============================================================================

output "public_ip" {
  description = <<-EOT
    Public IPv4 address of the W1nWire server (reserved, therefore stable).
    This is the address clients connect to.
  EOT
  value       = oci_core_public_ip.w1nwire.ip_address
}

output "private_ip" {
  description = "Private IPv4 address of the instance inside the VCN."
  value       = oci_core_instance.w1nwire.private_ip
}

output "ssh_command" {
  description = "Ready-to-paste SSH command for the server."
  value = join(" ", [
    "ssh",
    "-i ~/.ssh/${var.project_name}_oracle",
    "${var.ssh_user}@${oci_core_public_ip.w1nwire.ip_address}",
  ])
}

output "server_endpoint" {
  description = "host:port pair that clients have to dial (W1nWire speaks UDP)."
  value       = "${oci_core_public_ip.w1nwire.ip_address}:${var.vpn_port}"
}

output "instance_ocid" {
  description = "OCID of the compute instance (useful for `oci compute instance ...`)."
  value       = oci_core_instance.w1nwire.id
}

output "vcn_ocid" {
  description = "OCID of the VCN created for W1nWire."
  value       = oci_core_vcn.w1nwire.id
}

output "client_config_template" {
  description = <<-EOT
    Template of the client configuration file.

    The schema is the one of internal/config.ClientConfig. The `private_key`
    and `server.public_key` fields are placeholders - the server key pair is
    generated on the instance (never stored in this repository) and has to be
    copied over with `scp`. Use `deploy/scripts/make-client-conf.sh` to render
    a ready-to-use client.json.
  EOT
  value = jsonencode({
    server = {
      address    = "${oci_core_public_ip.w1nwire.ip_address}:${var.vpn_port}"
      public_key = "BASE64_SERVER_PUBLIC_KEY"
    }
    private_key = "BASE64_CLIENT_PRIVATE_KEY"
    tun = {
      enabled          = true
      name             = var.tun_interface
      address          = "${local.tunnel_client_cidr}"
      gateway          = local.tunnel_server_ip
      mtu              = var.tun_mtu
      configure_routes = true
      route_all        = false
    }
    socks5 = {
      enabled = true
      listen  = "127.0.0.1:1080"
    }
    dns = "1.1.1.1"
    http = {
      metrics = {
        enabled = true
        host    = "127.0.0.1"
        port    = 9188
      }
    }
  })
}

output "next_steps" {
  description = "Human readable checklist of the remaining deployment steps."
  value = <<-EOT
    W1nWire server is up.

    1. Fetch the configuration of the first client. The bootstrap generated a
       client key pair, registered it as the only peer of the server and wrote
       the matching configuration:

         ssh -i ~/.ssh/${var.project_name}_oracle ${var.ssh_user}@${oci_core_public_ip.w1nwire.ip_address} \\
           'sudo cat /etc/w1nwire/client.json' > client.json

       The public key of the server (needed for further clients) is fetched
       the same way:

         ssh -i ~/.ssh/${var.project_name}_oracle ${var.ssh_user}@${oci_core_public_ip.w1nwire.ip_address} \\
           'sudo cat /etc/w1nwire/server.pub' > server.pub

    2. Install the client on the client machine:

         sudo mkdir -p /etc/w1nwire
         sudo cp client.json /etc/w1nwire/client.json
         sudo cp ./deploy/systemd/w1nwire-client.service /etc/systemd/system/
         sudo systemctl daemon-reload
         sudo systemctl enable --now w1nwire-client

    3. Add further clients on your workstation:

         ./deploy/scripts/gen-keys.sh --out ./keys --name laptop
         ./deploy/scripts/make-client-conf.sh \\
           --server-host ${oci_core_public_ip.w1nwire.ip_address} \\
           --server-port ${var.vpn_port} \\
           --client-private-key ./keys/laptop.key \\
           --server-public-key ./server.pub \\
           --client-public-key ./keys/laptop.pub \\
           --out laptop.json

       The script writes laptop.entry.json as well - add that object to the
       "peers" array of /etc/w1nwire/server.json on the server and restart it:

         ssh -i ~/.ssh/${var.project_name}_oracle ${var.ssh_user}@${oci_core_public_ip.w1nwire.ip_address} \\
           'sudo systemctl restart w1nwire-server'

    4. Verify the server log:

         ssh -i ~/.ssh/${var.project_name}_oracle ${var.ssh_user}@${oci_core_public_ip.w1nwire.ip_address} \\
           'sudo journalctl -u w1nwire-server -n 50 --no-pager'
  EOT
}
