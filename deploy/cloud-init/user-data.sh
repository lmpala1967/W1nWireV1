#!/usr/bin/env bash
# =============================================================================
# W1nWire — cloud-init bootstrap for the OCI Always Free server.
#
# This file is a *Terraform template*: it is rendered with `templatefile()`
# and base64 encoded into the instance metadata by `deploy/terraform/main.tf`.
#
#   * `$${name}` -> a value provided by Terraform (see main.tf).
#   * `$name`   -> a plain shell variable (never expanded by Terraform).
#
# The script is fully IDEMPOTENT: it can be re-run at any time (for example
# `cloud-init clean --logs --reboot` or by hand) without breaking a working
# installation. Every step checks for the desired state before changing it.
#
# It performs the following tasks:
#   1. detects the distribution (Debian/Ubuntu vs. Oracle Linux)
#   2. installs the base packages (curl, git, iptables, openssl, cron, ...)
#   3. installs the Go toolchain (official tarball, SHA-256 verified)
#   4. fetches the sources (git clone -> pre-uploaded tarball -> stub)
#   5. builds w1nwire-server / w1nwire-client
#   6. creates the system user, the directories and the X25519 key pair
#   7. writes /etc/w1nwire/server.json and the systemd units
#   8. enables IPv4/IPv6 forwarding and the NAT + firewall rules
#   9. installs the idle-keepalive cron job and a weekly update timer
# =============================================================================

set -euo pipefail

# --- Values rendered by Terraform ---------------------------------------------
GO_VERSION="${go_version}"
GO_SHA256_AMD64="${go_sha256_amd64}"
GO_SHA256_ARM64="${go_sha256_arm64}"
REPO_URL="${repo_url}"
REPO_BRANCH="${repo_branch}"
VPN_PORT="${vpn_port}"
TUNNEL_CIDR="${tunnel_cidr}"
TUN_SERVER_CIDR="${tunnel_server_cidr}"
TUN_SERVER_IP="${tunnel_server_ip}"
TUN_CLIENT_CIDR="${tunnel_client_cidr}"
TUN_CLIENT_IP="${tunnel_client_ip}"
TUN_MTU="${tun_mtu}"
TUN_IFACE="${tun_interface}"
SSH_CIDR="${allowed_ssh_cidr}"
SERVER_CMD="${server_cmd_path}"
CLIENT_CMD="${client_cmd_path}"
INSTALL_KEEPALIVE="${install_keepalive}"

# --- Paths and constants -----------------------------------------------------
APP_DIR="/opt/w1nwire"
SRC_DIR="/opt/w1nwire/src"
BIN_DIR="/opt/w1nwire/bin"
CONF_DIR="/etc/w1nwire"
STATE_DIR="/var/lib/w1nwire"
RUN_DIR="/run/w1nwire"
SRC_TARBALL="/opt/w1nchi/w1nwire-src.tar.gz"
LOG_FILE="/var/log/w1nwire-bootstrap.log"
SVC_USER="w1nwire"
SVC_GROUP="w1nwire"
WAN_IF=""

# --- Logging ------------------------------------------------------------------
mkdir -p "$(dirname "$LOG_FILE")"
log() {
  printf '%s [w1nwire-bootstrap] %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*" | tee -a "$LOG_FILE"
}
die() {
  log "FATAL: $*"
  exit 1
}

log "bootstrap started (repo=$REPO_URL branch=$REPO_BRANCH port=$VPN_PORT)"

if [ "$(id -u)" -ne 0 ]; then
  die "this bootstrap must run as root"
fi

# =============================================================================
# 1. Distribution detection
# =============================================================================
OS_ID=$(grep -E '^ID=' /etc/os-release 2>/dev/null | cut -d= -f2 | tr -d '"' || true)
OS_VERSION_ID=$(grep -E '^VERSION_ID=' /etc/os-release 2>/dev/null | cut -d= -f2 | tr -d '"' || true)

case "$OS_ID" in
  ubuntu | debian)
    PKG_FAMILY="deb"
    ;;
  ol | oracle | rhel | centos | rocky | almalinux)
    PKG_FAMILY="rpm"
    ;;
  *)
    log "WARNING: unknown distribution '$OS_ID' ($OS_VERSION_ID) - assuming Debian-like"
    PKG_FAMILY="deb"
    ;;
esac
log "detected OS: $OS_ID $OS_VERSION_ID (package family: $PKG_FAMILY)"

# =============================================================================
# 2. Base packages
# =============================================================================
install_base_packages() {
  case "$PKG_FAMILY" in
    deb)
      export DEBIAN_FRONTEND=noninteractive
      # Pre-seed iptables-persistent so the installation does not block on
      # a debconf dialog (cloud-init has no terminal to answer it).
      echo "iptables-persistent iptables-persistent/autoinstall_v4 boolean true" | debconf-set-selections || true
      echo "iptables-persistent iptables-persistent/autoinstall_v6 boolean true" | debconf-set-selections || true
      apt-get update -qq
      apt-get install -y --no-install-recommends \
        ca-certificates curl git tar gzip openssl \
        iptables cron iptables-persistent
      # ufw would rewrite the INPUT chain and fight with the W1nWire rules.
      if command -v ufw > /dev/null 2>&1; then
        ufw --force disable > /dev/null 2>&1 || true
      fi
      ;;
    rpm)
      if command -v dnf > /dev/null 2>&1; then
        dnf install -y ca-certificates curl git tar gzip openssl iptables iptables-services cronie crontabs
      else
        yum install -y ca-certificates curl git tar gzip openssl iptables iptables-services cronie crontabs
      fi
      # firewalld rewrites the firewall as well - the W1nWire chain manages it.
      if command -v firewall-cmd > /dev/null 2>&1; then
        systemctl disable --now firewalld > /dev/null 2>&1 || true
      fi
      ;;
  esac
}

log "installing base packages"
install_base_packages
log "base packages ready"

# =============================================================================
# 3. Go toolchain (official tarball, checksum verified)
# =============================================================================
install_go() {
  local arch sha url current

  arch=$(uname -m)
  case "$arch" in
    x86_64 | amd64)
      arch="amd64"
      sha="$GO_SHA256_AMD64"
      ;;
    aarch64 | arm64)
      arch="arm64"
      sha="$GO_SHA256_ARM64"
      ;;
    *)
      die "unsupported CPU architecture '$arch'"
      ;;
  esac

  if [ -x /usr/local/go/bin/go ]; then
    current=$(/usr/local/go/bin/go env GOVERSION 2>/dev/null || echo "")
    if [ "$current" = "go$GO_VERSION" ]; then
      log "Go $GO_VERSION already installed - skipping"
      return 0
    fi
    log "replacing Go $current with Go $GO_VERSION"
  fi

  url="https://go.dev/dl/go$GO_VERSION.linux-$arch.tar.gz"
  log "downloading $url"
  curl -fsSL --retry 5 --retry-delay 5 --retry-connrefused "$url" -o /tmp/go-toolchain.tar.gz
  echo "$sha  /tmp/go-toolchain.tar.gz" | sha256sum -c - || die "Go tarball checksum mismatch"
  rm -rf /usr/local/go
  tar -C /usr/local -xzf /tmp/go-toolchain.tar.gz
  ln -sf /usr/local/go/bin/go /usr/local/bin/go
  ln -sf /usr/local/go/bin/gofmt /usr/local/bin/gofmt
  rm -f /tmp/go-toolchain.tar.gz
  log "Go $GO_VERSION installed"
}

log "installing Go toolchain"
install_go
export PATH="/usr/local/go/bin:$PATH"

# =============================================================================
# 4. Sources: git clone -> pre-uploaded tarball -> stub
# =============================================================================
mkdir -p "$APP_DIR" "$SRC_DIR" "$BIN_DIR"

acquire_sources() {
  local branch_ok=0

  if [ -n "$REPO_URL" ]; then
    if [ -d "$SRC_DIR/.git" ]; then
      log "updating existing checkout in $SRC_DIR"
      git -C "$SRC_DIR" fetch --tags --force --prune || true
      git -C "$SRC_DIR" checkout --force "$REPO_BRANCH" || true
      git -C "$SRC_DIR" pull --ff-only || true
      branch_ok=1
    else
      rm -rf "$SRC_DIR"
      if git clone --depth 1 --branch "$REPO_BRANCH" "$REPO_URL" "$SRC_DIR" 2>> "$LOG_FILE"; then
        branch_ok=1
      else
        log "git clone of branch '$REPO_BRANCH' failed - trying the default branch"
        rm -rf "$SRC_DIR"
        if git clone --depth 1 "$REPO_URL" "$SRC_DIR" 2>> "$LOG_FILE"; then
          branch_ok=1
        else
          log "WARNING: unable to clone $REPO_URL"
          rm -rf "$SRC_DIR"
        fi
      fi
    fi
  else
    log "no repository configured - looking for a pre-uploaded tarball"
  fi

  if [ "$branch_ok" -eq 0 ] && [ -f "$SRC_TARBALL" ]; then
    log "falling back to the pre-uploaded tarball $SRC_TARBALL"
    rm -rf "$SRC_DIR"
    mkdir -p "$SRC_DIR"
    tar -C "$SRC_DIR" --strip-components=1 -xzf "$SRC_TARBALL" || die "cannot extract $SRC_TARBALL"
    branch_ok=1
  fi

  if [ "$branch_ok" -eq 1 ]; then
    log "sources are available in $SRC_DIR"
    return 0
  fi

  log "ERROR: no sources available - the server service will fail with a clear message"
  return 1
}

acquire_sources || true

# =============================================================================
# 5. Build
# =============================================================================
# The command packages live in cmd/ (written by the application part of the
# project). If the configured path does not exist the closest match inside
# cmd/ is used, so that a slightly different layout still builds.
resolve_package() {
  local configured="$1" pattern="$2" candidate
  if [ -d "$SRC_DIR/$configured" ]; then
    echo "$configured"
    return 0
  fi
  for candidate in "$SRC_DIR"/cmd/*"$pattern"*; do
    if [ -d "$candidate" ]; then
      echo "$candidate" | sed -e "s|^$SRC_DIR/||"
      return 0
    fi
  done
  return 1
}

build_binary() {
  local configured="$1" pattern="$2" output="$3" pkg
  pkg=$(resolve_package "$configured" "$pattern") || {
    log "no package matching '$pattern' found under cmd/ - skipping $output"
    return 1
  }
  log "building $pkg -> $output"
  (
    cd "$SRC_DIR"
    /usr/local/go/bin/go build -trimpath -ldflags "-s -w" -o "$output" "$pkg"
  ) || {
    log "ERROR: build of $pkg failed"
    return 1
  }
  chmod 0755 "$output"
  return 0
}

BUILD_OK=0
if [ -d "$SRC_DIR" ]; then
  if build_binary "$SERVER_CMD" "server" "$BIN_DIR/w1nwire-server"; then
    BUILD_OK=1
  fi
  build_binary "$CLIENT_CMD" "client" "$BIN_DIR/w1nwire-client" || true
fi

if [ "$BUILD_OK" -ne 1 ]; then
  # The service unit expects the binary at this exact path. Install a stub
  # that explains the problem instead of failing with "no such file".
  log "installing the explanatory stub for $BIN_DIR/w1nwire-server"
  cat > "$BIN_DIR/w1nwire-server" <<'STUB_EOF'
#!/bin/sh
# W1nWire server stub.
#
# The real binary could not be built during the cloud-init bootstrap, most
# likely because the source repository was not reachable from this instance.
#
# How to fix it:
#   1. make the repository reachable (git clone / upload the tarball to
#      /opt/w1nchi/w1nwire-src.tar.gz), and
#   2. re-run the bootstrap:
#        sudo /opt/w1nwire/bin/w1nwire-build.sh
echo "w1nwire-server: the server binary was not built on this instance." >&2
echo "w1nwire-server: sources are missing or the build failed - see /var/log/w1nwire-bootstrap.log" >&2
exit 1
STUB_EOF
  chmod 0755 "$BIN_DIR/w1nwire-server"
fi

# =============================================================================
# 6. System user, directories, key material
# =============================================================================
if ! id -u "$SVC_USER" > /dev/null 2>&1; then
  useradd --system --home-dir "$STATE_DIR" --shell /usr/sbin/nologin "$SVC_USER"
  log "created system user $SVC_USER"
fi
getent group "$SVC_GROUP" > /dev/null 2>&1 || groupadd --system "$SVC_GROUP"

mkdir -p "$CONF_DIR" "$STATE_DIR" "$RUN_DIR"
chmod 750 "$CONF_DIR"
chown -R "$SVC_USER:$SVC_GROUP" "$STATE_DIR" "$RUN_DIR"

# --- X25519 key pair ----------------------------------------------------------
# Format: the raw 32 byte keys (RFC 7748 X25519) encoded with base64 - the
# very same format the `w1nwire-server keygen` command produces.
#
# The server needs its own key pair and - so that the instance is usable right
# after the bootstrap - the key pair of the first client ("bootstrap"), which
# is registered as the only peer in the server configuration.
generate_x25519_keypair() {
  local prefix="$1" pem
  pem=$(mktemp)
  openssl genpkey -algorithm X25519 -out "$pem" 2>> "$LOG_FILE"
  openssl pkey -in "$pem" -outform DER 2>/dev/null | tail -c 32 | base64 | tr -d '\n' > "$prefix.key"
  openssl pkey -in "$pem" -pubout -outform DER 2>/dev/null | tail -c 32 | base64 | tr -d '\n' > "$prefix.pub"
  rm -f "$pem"
}

if [ ! -s "$CONF_DIR/server.key" ] || [ ! -s "$CONF_DIR/server.pub" ]; then
  log "generating the X25519 server key pair"
  generate_x25519_keypair "$CONF_DIR/server"
else
  log "server key pair already present - keeping it"
fi

if [ ! -s "$CONF_DIR/client.key" ] || [ ! -s "$CONF_DIR/client.pub" ]; then
  log "generating the X25519 key pair of the first client"
  generate_x25519_keypair "$CONF_DIR/client"
else
  log "client key pair already present - keeping it"
fi

chmod 0600 "$CONF_DIR/server.key" "$CONF_DIR/client.key"
chmod 0644 "$CONF_DIR/server.pub" "$CONF_DIR/client.pub"

SERVER_KEY=$(tr -d '\n' < "$CONF_DIR/server.key")
SERVER_PUB=$(tr -d '\n' < "$CONF_DIR/server.pub")
CLIENT_KEY=$(tr -d '\n' < "$CONF_DIR/client.key")
CLIENT_PUB=$(tr -d '\n' < "$CONF_DIR/client.pub")

# =============================================================================
# 7. Configuration files and systemd units
# =============================================================================
detect_wan_interface() {
  local iface
  iface=$(ip -4 route get 8.8.8.8 2>/dev/null | awk '{for (i = 1; i <= NF; i++) if ($i == "dev") {print $(i + 1); exit}}' || true)
  if [ -z "$iface" ]; then
    iface="ens3"
    log "WARNING: could not detect the uplink interface - assuming $iface"
  fi
  echo "$iface"
}
WAN_IF=$(detect_wan_interface)
log "uplink interface: $WAN_IF"

# --- public IP ----------------------------------------------------------------
# The OCI metadata service knows the public address of the instance. It is
# used to fill the endpoint of the generated client configuration.
detect_public_ip() {
  local raw
  raw=$(curl -fsS --max-time 5 -H "Authorization: Bearer Oracle" \
    http://169.254.169.254/opc/v2/instance/ 2> /dev/null || true)
  [ -n "$raw" ] || return 1
  printf '%s' "$raw" \
    | grep -o '"publicIp"[[:space:]]*:[[:space:]]*"[^"]*"' \
    | head -n 1 \
    | sed 's/.*:[[:space:]]*"\([^"]*\)"/\1/'
}
PUBLIC_IP=$(detect_public_ip || true)
if [ -z "$PUBLIC_IP" ]; then
  PUBLIC_IP="REPLACE_WITH_THE_PUBLIC_IP"
  log "WARNING: the public IP could not be detected - the client configuration contains a placeholder"
else
  log "public IP: $PUBLIC_IP"
fi

# --- server.json --------------------------------------------------------------
# The schema is the one of internal/config.ServerConfig. The JSON decoder of
# the application rejects unknown fields, therefore the file must match it
# exactly.
#
# NOTE on "nat": the masquerading rule and the forwarding rules are installed
# by the bootstrap itself (see section 9). The application would install them
# as well, but its implementation starts with `sysctl -w net.ipv4.ip_forward=1`
# which would require CAP_SYS_ADMIN - a capability the service deliberately
# does not have.
if [ ! -f "$CONF_DIR/server.json" ]; then
  log "writing $CONF_DIR/server.json"
  cat > "$CONF_DIR/server.json" <<JSON_EOF
{
  "listen": "0.0.0.0:$VPN_PORT",
  "server_private_key": "$SERVER_KEY",
  "tun": {
    "enabled": true,
    "name": "$TUN_IFACE",
    "address": "$TUN_SERVER_CIDR",
    "mtu": $TUN_MTU
  },
  "nat": {
    "enabled": false,
    "outbound_interface": "$WAN_IF"
  },
  "http": {
    "metrics": {
      "enabled": true,
      "host": "127.0.0.1",
      "port": 9187
    }
  },
  "peers": [
    {
      "id": "bootstrap",
      "public_key": "$CLIENT_PUB",
      "tunnel_address": "$TUN_CLIENT_IP",
      "allowed_ips": ["$TUNNEL_CIDR"]
    }
  ]
}
JSON_EOF
  chmod 0600 "$CONF_DIR/server.json"
  chown "$SVC_USER:$SVC_GROUP" "$CONF_DIR/server.json"
else
  log "$CONF_DIR/server.json already exists - keeping it"
fi

# --- client.json --------------------------------------------------------------
# A ready to use configuration of the first client. The endpoint is filled
# with the public IP of the instance, so that the file can be copied to a
# client machine as it is.
if [ ! -f "$CONF_DIR/client.json" ]; then
  log "writing $CONF_DIR/client.json"
  cat > "$CONF_DIR/client.json" <<CLIENT_JSON_EOF
{
  "server": {
    "address": "$PUBLIC_IP:$VPN_PORT",
    "public_key": "$SERVER_PUB"
  },
  "private_key": "$CLIENT_KEY",
  "tun": {
    "enabled": true,
    "name": "w1n0",
    "address": "$TUN_CLIENT_CIDR",
    "gateway": "$TUN_SERVER_IP",
    "mtu": $TUN_MTU,
    "configure_routes": true,
    "route_all": false
  },
  "socks5": {
    "enabled": true,
    "listen": "127.0.0.1:1080"
  },
  "dns": "1.1.1.1",
  "http": {
    "metrics": {
      "enabled": true,
      "host": "127.0.0.1",
      "port": 9188
    }
  }
}
CLIENT_JSON_EOF
  chmod 0600 "$CONF_DIR/client.json"
  chown "$SVC_USER:$SVC_GROUP" "$CONF_DIR/client.json"
else
  log "$CONF_DIR/client.json already exists - keeping it"
fi

# --- server.env ---------------------------------------------------------------
cat > "$CONF_DIR/server.env" <<ENV_EOF
W1NWIRE_PORT=$VPN_PORT
W1NWIRE_TUNNEL_CIDR=$TUNNEL_CIDR
W1NWIRE_TUN_IFACE=$TUN_IFACE
W1NWIRE_CONFIG=$CONF_DIR/server.json
W1NWIRE_STATE_DIR=$STATE_DIR
ENV_EOF
chmod 0640 "$CONF_DIR/server.env"
chown "$SVC_USER:$SVC_GROUP" "$CONF_DIR/server.env"

# --- update.env (used by the weekly update job) -------------------------------
cat > "$CONF_DIR/update.env" <<UPDATE_ENV_EOF
REPO_URL=$REPO_URL
REPO_BRANCH=$REPO_BRANCH
GO_VERSION=$GO_VERSION
SRC_DIR=$SRC_DIR
BIN_DIR=$BIN_DIR
UPDATE_ENV_EOF
chmod 0644 "$CONF_DIR/update.env"

# --- systemd units ------------------------------------------------------------
# The unit files are rendered from deploy/systemd/* and injected by Terraform,
# so there is exactly one source of truth for them.
install_unit() {
  local name="$1" content="$2" target="/etc/systemd/system/$1"
  printf '%s\n' "$content" > "$target"
  chmod 0644 "$target"
  log "installed $target"
}

install_unit "w1nwire-server.service" "${server_unit}"
install_unit "w1nwire-client.service" "${client_unit}"

# --- build helper (used by the stub message and the update job) --------------
cat > "$BIN_DIR/w1nwire-build.sh" <<'BUILD_EOF'
#!/usr/bin/env bash
# Rebuild the W1nWire binaries from the sources in $SRC_DIR.
set -euo pipefail
SRC_DIR="/opt/w1nwire/src"
BIN_DIR="/opt/w1nwire/bin"
if [ -f /etc/w1nwire/update.env ]; then
  . /etc/w1nwire/update.env
fi
if [ ! -d "$SRC_DIR" ]; then
  echo "no sources in $SRC_DIR" >&2
  exit 1
fi
cd "$SRC_DIR"
for pkg in ./cmd/*server* ./cmd/*client*; do
  [ -d "$pkg" ] || continue
  case "$pkg" in
    *server*) out="$BIN_DIR/w1nwire-server" ;;
    *) out="$BIN_DIR/w1nwire-client" ;;
  esac
  /usr/local/go/bin/go build -trimpath -ldflags "-s -w" -o "$out" "$pkg"
  echo "built $out"
done
systemctl restart w1nwire-server.service || true
BUILD_EOF
chmod 0755 "$BIN_DIR/w1nwire-build.sh"

systemctl daemon-reload

# =============================================================================
# 8. Kernel: forwarding, TUN device
# =============================================================================
log "enabling IPv4/IPv6 forwarding"
cat > /etc/sysctl.d/99-w1nwire.conf <<'SYSCTL_EOF'
# W1nWire - required for the tunnel to route traffic between clients.
net.ipv4.ip_forward = 1
net.ipv6.conf.all.forwarding = 1
net.ipv6.conf.default.forwarding = 1
# Larger UDP buffers for the custom protocol (handshake + data packets).
net.core.rmem_max = 2621440
net.core.wmem_max = 2621440
SYSCTL_EOF
sysctl --system > /dev/null 2>&1 || sysctl -p /etc/sysctl.d/99-w1nwire.conf > /dev/null 2>&1 || true

if ! grep -q '^tun$' /etc/modules-load.d/tun.conf 2>/dev/null; then
  mkdir -p /etc/modules-load.d
  echo "tun" > /etc/modules-load.d/tun.conf
fi
modprobe tun 2> /dev/null || true
if [ ! -c /dev/net/tun ]; then
  mkdir -p /dev/net
  mknod /dev/net/tun c 10 200 || true
  chmod 0666 /dev/net/tun || true
fi

# =============================================================================
# 9. NAT + firewall
#
# The rules are installed idempotently: every rule is checked with
# `iptables -C` before it is appended, so a re-run never duplicates them.
#
#   * NAT      : masquerade the tunnel network behind the public IP
#   * FORWARD  : allow tun -> wan and the established return path
#   * INPUT    : a dedicated W1NWIRE_INPUT chain allows SSH, the VPN port,
#                DHCP, ICMP (PMTU) and the OCI metadata service; everything
#                else is dropped.
# =============================================================================
configure_networking() {
  # --- NAT -------------------------------------------------------------------
  if ! iptables -t nat -C POSTROUTING -s "$TUNNEL_CIDR" -o "$WAN_IF" -j MASQUERADE 2> /dev/null; then
    iptables -t nat -A POSTROUTING -s "$TUNNEL_CIDR" -o "$WAN_IF" -j MASQUERADE
    log "added NAT rule for $TUNNEL_CIDR via $WAN_IF"
  fi

  # --- FORWARD ---------------------------------------------------------------
  if ! iptables -C FORWARD -i "$TUN_IFACE" -o "$WAN_IF" -j ACCEPT 2> /dev/null; then
    iptables -A FORWARD -i "$TUN_IFACE" -o "$WAN_IF" -j ACCEPT
  fi
  if ! iptables -C FORWARD -i "$WAN_IF" -o "$TUN_IFACE" -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT 2> /dev/null; then
    iptables -A FORWARD -i "$WAN_IF" -o "$TUN_IFACE" -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT
  fi
  if ! iptables -C FORWARD -i "$TUN_IFACE" -o "$TUN_IFACE" -j ACCEPT 2> /dev/null; then
    iptables -A FORWARD -i "$TUN_IFACE" -o "$TUN_IFACE" -j ACCEPT
  fi
  # Clamp the MSS to the path MTU - without it the handshake breaks as soon
  # as a client sits behind a DSL line with a smaller MTU.
  if ! iptables -t mangle -C FORWARD -p tcp --tcp-flags SYN,RST SYN -j TCPMSS --clamp-mss-to-pmtu 2> /dev/null; then
    iptables -t mangle -A FORWARD -p tcp --tcp-flags SYN,RST SYN -j TCPMSS --clamp-mss-to-pmtu
  fi

  # --- INPUT (host firewall) --------------------------------------------------
  iptables -N W1NWIRE_INPUT 2> /dev/null || true
  iptables -F W1NWIRE_INPUT
  iptables -A W1NWIRE_INPUT -i lo -j ACCEPT
  iptables -A W1NWIRE_INPUT -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT
  # OCI metadata service (169.254.169.254) - the cloud agent lives there.
  iptables -A W1NWIRE_INPUT -s 169.254.0.0/16 -j ACCEPT
  # DHCP renewals of the instance VNIC (UDP 68).
  iptables -A W1NWIRE_INPUT -p udp --dport 68 -j ACCEPT
  # ICMP is required for path MTU discovery and ping diagnostics.
  iptables -A W1NWIRE_INPUT -p icmp -j ACCEPT
  iptables -A W1NWIRE_INPUT -p icmpv6 -j ACCEPT
  # SSH (restricted to the configured CIDR).
  iptables -A W1NWIRE_INPUT -p tcp --dport 22 -s "$SSH_CIDR" -j ACCEPT
  # The W1nWire protocol itself.
  iptables -A W1NWIRE_INPUT -p udp --dport "$VPN_PORT" -j ACCEPT
  # Everything else is denied.
  iptables -A W1NWIRE_INPUT -j DROP

  if ! iptables -C INPUT -j W1NWIRE_INPUT 2> /dev/null; then
    iptables -I INPUT 1 -j W1NWIRE_INPUT
    log "installed the W1NWIRE_INPUT firewall chain"
  fi
}

persist_firewall() {
  case "$PKG_FAMILY" in
    deb)
      mkdir -p /etc/iptables
      iptables-save > /etc/iptables/rules.v4
      systemctl enable netfilter-persistent > /dev/null 2>&1 || true
      ;;
    rpm)
      iptables-save > /etc/sysconfig/iptables
      systemctl enable iptables > /dev/null 2>&1 || true
      ;;
  esac
}

log "configuring NAT and the firewall"
configure_networking
persist_firewall

# =============================================================================
# 10. Idle keepalive (protects the Always Free instance from reclamation)
# =============================================================================
install_keepalive() {
  cat > /usr/local/bin/oracle-idle-keepalive.sh <<'KEEPALIVE_EOF'
${keepalive_script}
KEEPALIVE_EOF
  chmod 0755 /usr/local/bin/oracle-idle-keepalive.sh

  local current
  current=$(crontab -l 2> /dev/null || true)
  printf '%s\n' "$current" | grep -v -e 'oracle-idle-keepalive' -e '^$' > /tmp/w1nwire.cron || true
  # Minute 17 of every 6th hour - off the top of the hour to avoid the
  # "same minute" stampede of every other cron job on the planet.
  echo "17 */6 * * * /usr/local/bin/oracle-idle-keepalive.sh >> /var/log/w1nwire-keepalive.log 2>&1" >> /tmp/w1nwire.cron
  crontab /tmp/w1nwire.cron
  rm -f /tmp/w1nwire.cron

  systemctl enable cron > /dev/null 2>&1 || systemctl enable crond > /dev/null 2>&1 || true
  systemctl start cron > /dev/null 2>&1 || systemctl start crond > /dev/null 2>&1 || true
  log "idle keepalive installed (cron: every 6 hours)"
}

if [ "$INSTALL_KEEPALIVE" = "true" ]; then
  install_keepalive
else
  log "idle keepalive disabled by configuration"
fi

# =============================================================================
# 11. Weekly self-update
# =============================================================================
install_update_job() {
  cat > /usr/local/bin/w1nwire-update.sh <<'UPDATE_EOF'
#!/usr/bin/env bash
# =============================================================================
# W1nWire - weekly maintenance job.
#
#   1. installs the pending OS security updates
#   2. pulls the newest sources (when a repository is configured)
#   3. rebuilds the binaries and restarts the service
#
# The script is intentionally conservative: every step may fail without
# leaving the instance in a broken state - the previous binaries are kept
# until the new ones are built successfully.
# =============================================================================
set -o pipefail

LOG_FILE="/var/log/w1nwire-update.log"
exec >> "$LOG_FILE" 2>&1
echo "--- $(date -u +%Y-%m-%dT%H:%M:%SZ) w1nwire update started ---"

# Load the deployment specific settings (REPO_URL, REPO_BRANCH, SRC_DIR...).
if [ -f /etc/w1nwire/update.env ]; then
  . /etc/w1nwire/update.env
fi
if [ -z "$SRC_DIR" ]; then
  SRC_DIR="/opt/w1nwire/src"
fi
if [ -z "$BIN_DIR" ]; then
  BIN_DIR="/opt/w1nwire/bin"
fi
if [ -z "$REPO_BRANCH" ]; then
  REPO_BRANCH="main"
fi

# 1) OS updates.
if command -v apt-get > /dev/null 2>&1; then
  apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get -y -o Dpkg::Options::=--force-confold upgrade
elif command -v dnf > /dev/null 2>&1; then
  dnf -y update
elif command -v yum > /dev/null 2>&1; then
  yum -y update
fi

# 2) Sources.
if [ -n "$REPO_URL" ] && [ -d "$SRC_DIR/.git" ]; then
  git -C "$SRC_DIR" fetch --tags --force --prune || true
  git -C "$SRC_DIR" checkout --force "$REPO_BRANCH" || true
  git -C "$SRC_DIR" pull --ff-only || true
fi

# 3) Rebuild + restart.
if [ -d "$SRC_DIR" ]; then
  cd "$SRC_DIR" || exit 0
  for pkg in ./cmd/*server*; do
    [ -d "$pkg" ] || continue
    if /usr/local/go/bin/go build -trimpath -ldflags "-s -w" -o "$BIN_DIR/w1nwire-server.new" "$pkg"; then
      mv "$BIN_DIR/w1nwire-server.new" "$BIN_DIR/w1nwire-server"
      systemctl restart w1nwire-server.service || true
    fi
  done
fi

echo "--- $(date -u +%Y-%m-%dT%H:%M:%SZ) w1nwire update finished ---"
UPDATE_EOF
  chmod 0755 /usr/local/bin/w1nwire-update.sh

  cat > /etc/systemd/system/w1nwire-update.service <<'UPDATE_UNIT_EOF'
[Unit]
Description=W1nWire weekly update (OS packages + sources + rebuild)
Documentation=https://github.com/w1nchi/w1nwire

[Service]
Type=oneshot
ExecStart=/usr/local/bin/w1nwire-update.sh
# The update must never take the instance down.
TimeoutStartSec=1800
UPDATE_UNIT_EOF

  cat > /etc/systemd/system/w1nwire-update.timer <<'UPDATE_TIMER_EOF'
[Unit]
Description=Weekly W1nWire update timer

[Timer]
OnCalendar=weekly
# Spread the load - do not start exactly at midnight.
RandomizedDelaySec=1h
Persistent=true

[Install]
WantedBy=timers.target
UPDATE_TIMER_EOF

  chmod 0644 /etc/systemd/system/w1nwire-update.service /etc/systemd/system/w1nwire-update.timer
  systemctl daemon-reload
  systemctl enable --now w1nwire-update.timer > /dev/null 2>&1 || true
  log "weekly update timer installed"
}

install_update_job

# =============================================================================
# 12. Enable and start the service
# =============================================================================
systemctl daemon-reload
systemctl enable w1nwire-server.service > /dev/null 2>&1 || true
if ! systemctl restart w1nwire-server.service > /dev/null 2>&1; then
  log "WARNING: w1nwire-server did not start - inspect: journalctl -u w1nwire-server"
else
  log "w1nwire-server started"
fi

log "server public key (base64): $(cat "$CONF_DIR/server.pub" 2>/dev/null || echo 'unavailable')"
log "bootstrap finished successfully"
