#!/usr/bin/env bash
# =============================================================================
# W1nWire — server installer.
#
# Tested target: Ubuntu 22.04 LTS, Ubuntu 24.04 LTS, Debian 12 (amd64/arm64),
# including a freshly created Oracle Cloud Ampere A1 instance. The script only
# needs what such an image already has: bash, coreutils, apt, curl.
#
# Usage:
#   sudo bash install.sh [options]
#
# Options:
#   --port <n>            UDP port of the VPN                  (default 51820)
#   --repo <owner/repo>   GitHub repository with the releases  (default: $W1NWIRE_REPO or w1nchi/w1nwire)
#   --version <tag>       release to install, e.g. v1.2.3 or latest (default: latest)
#   --from-source         build the server from source with the local Go toolchain
#   --ios                 also set up a WireGuard endpoint for the iPhone (UDP 51821)
#   --no-ios              do not touch WireGuard                   (default)
#   --add-phone           add one more phone to an existing wg0 and print its QR code
#   --no-firewall         do not open the UDP port in ufw / the Oracle iptables chain
#   --no-keepalive        do not install the Oracle idle keepalive timer
#   --uninstall           stop and remove everything this script installs
#   -h, --help            this text
#
# Environment:
#   W1NWIRE_REPO          same as --repo
#   W1NWIRE_PUBLIC_HOST   override the host/IP written into the invite code
#   W1NWIRE_WAN_IF        override the detected outbound interface
#
# Every step is idempotent: re-running the script keeps the existing key, the
# existing server.json and every peer that was already added. It never asks a
# question; all decisions come from the flags.
#
# What it does:
#   1. checks root, systemd, distribution, architecture,
#   2. installs curl/python3/iptables/iproute2 only if they are missing,
#   3. downloads the release binary to /usr/local/bin/w1nwire-server (chmod 755)
#      and checks that it runs (`w1nwire-server version`),
#   4. generates /etc/w1nwire/server.key, writes /etc/w1nwire/server.json (0600),
#   5. creates the first peer and prints its invite code,
#   6. installs and starts systemd unit w1nwire-server (Restart=always),
#   7. enables IPv4 forwarding, installs the MASQUERADE/forward rules and
#      persists them; opens the UDP port in ufw and before Oracle's REJECT rule,
#   8. installs the Oracle idle keepalive timer,
#   9. with --ios: installs WireGuard, creates wg0 on UDP 51821 and prints a QR.
# =============================================================================
set -euo pipefail
umask 022

PROGRAM="${0##*/}"
SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"

# --- Options and defaults ----------------------------------------------------
PORT=51820
REPO="${W1NWIRE_REPO:-w1nchi/w1nwire}"
VERSION="latest"
PEER_NAME="desktop"

PORT_SET=0
WITH_IOS=0
FIREWALL=1
KEEPALIVE=1
FROM_SOURCE=0
DO_UNINSTALL=0
ADD_PHONE=0

# --- Paths -------------------------------------------------------------------
BIN_PATH=/usr/local/bin/w1nwire-server
CONF_DIR=/etc/w1nwire
CONF_PATH="$CONF_DIR/server.json"
KEY_PATH="$CONF_DIR/server.key"
INVITE_DIR="$CONF_DIR/invites"
UNIT_PATH=/etc/systemd/system/w1nwire-server.service
SYSCTL_PATH=/etc/sysctl.d/99-w1nwire.conf
KEEPALIVE_BIN_PATH=/usr/local/bin/w1nwire-keepalive.sh
KEEPALIVE_UNIT_PATH=/etc/systemd/system/w1nwire-keepalive.service
KEEPALIVE_TIMER_PATH=/etc/systemd/system/w1nwire-keepalive.timer
IPTABLES_RULES=/etc/iptables/rules.v4

# --- Tunnel defaults ---------------------------------------------------------
TUN_NAME="w1nwire0"
TUN_SERVER_ADDRESS="10.66.0.1/24"
TUN_SUBNET="10.66.0.0/24"
TUN_SERVER_IP="10.66.0.1"
TUN_MTU=1420

# --- iPhone / WireGuard endpoint --------------------------------------------
WG_IFACE="wg0"
WG_ADDRESS="10.67.0.1/24"
WG_SUBNET="10.67.0.0/24"
WG_PORT=51821

# --- Runtime state -----------------------------------------------------------
WORKDIR=""
ARCH=""
OS_ID=""
OS_VERSION=""
WAN_IF=""
PUBLIC_IP=""
INVITE=""
SERVER_PUBLIC_KEY=""
SERVER_PRIVATE_KEY=""
RESTART_NEEDED=0

# --- Output helpers ----------------------------------------------------------
if [ -t 1 ]; then
  C_RESET=$'\033[0m'
  C_STEP=$'\033[1;36m'
  C_INFO=$'\033[0;32m'
  C_WARN=$'\033[0;33m'
  C_ERR=$'\033[1;31m'
else
  C_RESET=""
  C_STEP=""
  C_INFO=""
  C_WARN=""
  C_ERR=""
fi

step() { printf '\n%s==> %s%s\n' "$C_STEP" "$*" "$C_RESET"; }
info() { printf '%s  - %s%s\n' "$C_INFO" "$*" "$C_RESET"; }
warn() { printf '%s  ! %s%s\n' "$C_WARN" "$*" "$C_RESET" >&2; }
error() { printf '%s  x %s%s\n' "$C_ERR" "$*" "$C_RESET" >&2; }
die() {
  error "$*"
  exit 1
}

cleanup() {
  if [ -n "${WORKDIR:-}" ] && [ -d "$WORKDIR" ]; then
    rm -rf -- "$WORKDIR"
  fi
}
trap cleanup EXIT

usage() {
  cat <<'USAGE'
W1nWire server installer (Ubuntu 22.04/24.04, Debian 12; amd64 and arm64)

Usage:
  sudo bash install.sh [options]

Options:
  --port <n>            UDP port of the VPN (default: 51820)
  --repo <owner/repo>   GitHub repository that publishes the release binaries
                        (default: $W1NWIRE_REPO, else w1nchi/w1nwire)
  --version <tag>       release to install: v1.2.3, 1.2.3 or latest (default: latest)
  --from-source         build w1nwire-server from source instead of downloading it
                        (needs git and a Go toolchain in PATH)
  --ios                 also install WireGuard and create wg0 on UDP 51821 for
                        the free WireGuard app on the iPhone; prints a QR code
  --no-ios              do not touch WireGuard (default)
  --add-phone           add one more phone to an existing wg0 and print its QR
  --no-firewall         do not open the UDP port in ufw or in the iptables INPUT
                        chain (NAT/forwarding is still installed)
  --no-keepalive        do not install the Oracle idle keepalive timer
  --uninstall           stop the services and remove the binary, the unit, the
                        configuration, the iptables rules and the keepalive
  -h, --help            this text

Examples:
  sudo bash install.sh
  sudo bash install.sh --port 443
  sudo bash install.sh --repo alice/W1nWire --version v0.2.0
  sudo bash install.sh --ios
  sudo bash install.sh --add-phone
  sudo bash install.sh --uninstall
  curl -fsSL https://raw.githubusercontent.com/alice/W1nWire/main/deploy/install.sh -o install.sh
  sudo bash install.sh

The script is idempotent: keys, /etc/w1nwire/server.json and existing peers are
kept. It never prompts.
USAGE
}

# --- Small utilities ---------------------------------------------------------
have() { command -v "$1" > /dev/null 2>&1; }

require_root() {
  if [ "$(id -u)" -ne 0 ]; then
    die "this script must run as root (try: sudo bash $PROGRAM)"
  fi
}

require_systemd() {
  have systemctl || die "systemd is required (systemctl not found)"
  if [ ! -d /run/systemd/system ]; then
    die "systemd does not appear to be running (is this a container?)"
  fi
}

detect_os() {
  if [ ! -r /etc/os-release ]; then
    die "/etc/os-release is missing - unsupported system"
  fi
  set +u
  # shellcheck disable=SC1091
  . /etc/os-release
  set -u
  OS_ID="${ID:-unknown}"
  OS_VERSION="${VERSION_ID:-unknown}"

  case "$OS_ID" in
    ubuntu)
      case "$OS_VERSION" in
        22.04 | 24.04) info "distribution: Ubuntu $OS_VERSION" ;;
        *) warn "Ubuntu $OS_VERSION is not a tested target (22.04 and 24.04 are); continuing" ;;
      esac
      ;;
    debian)
      case "$OS_VERSION" in
        12) info "distribution: Debian $OS_VERSION" ;;
        *) warn "Debian $OS_VERSION is not a tested target (12 is); continuing" ;;
      esac
      ;;
    *)
      warn "distribution '$OS_ID $OS_VERSION' is not a tested target; continuing because apt-get exists"
      ;;
  esac

  have apt-get || die "apt-get is required (this installer only supports Debian-based systems)"
}

detect_arch() {
  local machine
  machine=$(uname -m)
  case "$machine" in
    x86_64 | amd64) ARCH=amd64 ;;
    aarch64 | arm64) ARCH=arm64 ;;
    *) die "unsupported architecture '$machine' (releases exist for amd64 and arm64)" ;;
  esac
  info "architecture: $ARCH"
}

# --- Packages ----------------------------------------------------------------
APT_OPTS=(-y -qq --no-install-recommends
  -o Dpkg::Options::=--force-confdef
  -o Dpkg::Options::=--force-confold)

apt_install() {
  DEBIAN_FRONTEND=noninteractive NEEDRESTART_MODE=a \
    apt-get install "${APT_OPTS[@]}" "$@" > /dev/null
}

apt_update_once() {
  if [ "${APT_UPDATED:-0}" -eq 0 ]; then
    info "apt-get update"
    apt-get update -qq -o Acquire::Retries=3 > /dev/null || warn "apt-get update failed; continuing with the existing package lists"
    APT_UPDATED=1
  fi
}

ensure_base_packages() {
  local missing=()

  have curl || missing+=(curl ca-certificates)
  have python3 || missing+=(python3)
  have iptables || missing+=(iptables)
  have ip || missing+=(iproute2)
  have sysctl || missing+=(procps)
  have sha256sum || missing+=(coreutils)

  if [ "${#missing[@]}" -eq 0 ]; then
    info "the base packages are already installed"
    return 0
  fi

  step "installing base packages: ${missing[*]}"
  apt_update_once
  apt_install "${missing[@]}" || die "could not install ${missing[*]}"
  have curl || die "curl is still missing after the installation"
  have python3 || die "python3 is still missing after the installation"
  have iptables || die "iptables is still missing after the installation"
}

# --- Binary ------------------------------------------------------------------
resolve_release_tag() {
  if [ "$VERSION" != latest ]; then
    printf '%s\n' "$VERSION"
    return 0
  fi
  # The API is only used to know which tag "latest" currently is; the download
  # itself goes through releases/latest when the API is not reachable.
  curl -fsSL --max-time 15 "https://api.github.com/repos/${REPO}/releases/latest" 2> /dev/null \
    | python3 -c 'import json,sys; print(json.load(sys.stdin).get("tag_name",""))' 2> /dev/null \
    || true
}

release_base_url() {
  local tag="$1"
  if [ -n "$tag" ]; then
    printf 'https://github.com/%s/releases/download/%s' "$REPO" "$tag"
  else
    printf 'https://github.com/%s/releases/latest/download' "$REPO"
  fi
}

verify_checksum() {
  local tag="$1" asset="$2" base want got

  # Only verify assets whose name carries the platform: the combined
  # SHA256SUMS.txt of the release can contain several entries called
  # "w1nwire-server" (one per OS) and a basename match would then compare
  # against the wrong binary.
  case "$asset" in
    *-linux-*) ;;
    *)
      info "no platform-specific asset name, skipping the checksum check"
      return 0
      ;;
  esac

  base="$(release_base_url "$tag")"
  if ! curl -fsSL --max-time 30 -o "$WORKDIR/SHA256SUMS.txt" "$base/SHA256SUMS.txt" 2> /dev/null; then
    info "the release publishes no SHA256SUMS.txt, skipping the checksum check"
    return 0
  fi

  want=$(awk -v n="$asset" '{ p=$2; sub(/^\.\//, "", p); sub(/^\*/, "", p); if (p == n) { print $1; exit } }' "$WORKDIR/SHA256SUMS.txt")
  if [ -z "$want" ]; then
    info "no checksum entry for $asset, skipping the checksum check"
    return 0
  fi
  got=$(sha256sum "$WORKDIR/w1nwire-server" | awk '{print $1}')
  if [ "$want" != "$got" ]; then
    error "checksum mismatch for $asset"
    error "  expected: $want"
    error "  actual:   $got"
    return 1
  fi
  info "checksum verified"
}

download_release_binary() {
  local tag="$1" base asset url ok=0

  base="$(release_base_url "$tag")"
  for asset in "w1nwire-server-linux-${ARCH}" "w1nwire-server"; do
    url="$base/$asset"
    info "downloading $url"
    if curl -fsSL --retry 3 --retry-delay 2 --connect-timeout 15 --max-time 300 \
      -o "$WORKDIR/w1nwire-server" "$url" 2> /dev/null; then
      ok=1
      break
    fi
    warn "not available: $asset"
  done

  if [ "$ok" -ne 1 ]; then
    error "the release binary could not be downloaded from $base"
    error "check the repository name (--repo), the version (--version) and that the"
    error "release publishes w1nwire-server-linux-${ARCH}"
    return 1
  fi

  chmod 0755 "$WORKDIR/w1nwire-server"
  verify_checksum "$tag" "$asset" || return 1

  if ! "$WORKDIR/w1nwire-server" version > /dev/null 2>&1; then
    error "the downloaded file does not run on this machine (wrong architecture?)"
    return 1
  fi
  install -m 0755 -o root -g root "$WORKDIR/w1nwire-server" "$BIN_PATH"
  info "installed $("$BIN_PATH" version 2> /dev/null)"
}

build_from_source() {
  local src="$WORKDIR/src"
  local tag="$1"

  have git || die "--from-source needs git (apt-get install -y git)"
  have go || die "--from-source needs a Go toolchain in PATH (go >= 1.26 for this module)"

  step "building w1nwire-server from source"
  if [ -n "$tag" ] && [ "$tag" != latest ]; then
    git clone --depth 1 --branch "$tag" "https://github.com/${REPO}.git" "$src" > /dev/null 2>&1 \
      || die "git clone of ${REPO} at $tag failed"
  else
    git clone --depth 1 "https://github.com/${REPO}.git" "$src" > /dev/null 2>&1 \
      || die "git clone of ${REPO} failed"
  fi

  ( cd "$src" && CGO_ENABLED=0 GOOS=linux GOARCH="$ARCH" \
    go build -trimpath -ldflags "-s -w -X main.version=source" \
    -o "$WORKDIR/w1nwire-server" ./cmd/w1nwire-server ) \
    || die "the build failed"

  install -m 0755 -o root -g root "$WORKDIR/w1nwire-server" "$BIN_PATH"
  info "installed $("$BIN_PATH" version 2> /dev/null)"
}

install_binary() {
  local tag installed

  if [ "$FROM_SOURCE" -eq 1 ]; then
    build_from_source "$(resolve_release_tag)"
    return 0
  fi

  step "installing the server binary"
  tag="$(resolve_release_tag)"
  tag=$(printf '%s' "$tag" | tr -d '\r\n')

  if [ -n "$tag" ]; then
    info "release: $tag"
  else
    info "release: latest (the API could not be queried, using the /latest/download URL)"
  fi

  if [ -x "$BIN_PATH" ]; then
    installed="$("$BIN_PATH" version 2> /dev/null | awk '{print $2}' || true)"
    if [ -n "$tag" ] && [ "$installed" = "$tag" ]; then
      info "$BIN_PATH is already at $tag, keeping it"
      return 0
    fi
    if [ -n "$installed" ]; then
      info "replacing the installed version $installed"
    fi
  fi

  download_release_binary "$tag" || die "installation aborted, the existing binary (if any) was left untouched"
}

# --- Public address ----------------------------------------------------------
is_private_ip() {
  case "$1" in
    10.* | 192.168.* | 172.1[6-9].* | 172.2[0-9].* | 172.3[01].* | 127.*) return 0 ;;
    *) return 1 ;;
  esac
}

detect_public_ip() {
  local candidate=""

  if [ -n "${W1NWIRE_PUBLIC_HOST:-}" ]; then
    PUBLIC_IP="$W1NWIRE_PUBLIC_HOST"
    info "public address (from W1NWIRE_PUBLIC_HOST): $PUBLIC_IP"
    return 0
  fi

  candidate=$(curl -4 -fsS --max-time 8 https://api.ipify.org 2> /dev/null || true)
  if [ -z "$candidate" ]; then
    candidate=$(curl -4 -fsS --max-time 8 https://ifconfig.me/ip 2> /dev/null || true)
  fi
  if [ -z "$candidate" ]; then
    candidate=$(hostname -I 2> /dev/null | awk '{print $1}')
    if [ -n "$candidate" ]; then
      warn "the public IP could not be detected, using the first local address ($candidate)"
    fi
  fi
  if [ -z "$candidate" ]; then
    die "the public IP could not be detected; re-run with W1NWIRE_PUBLIC_HOST=<ip or host>"
  fi

  PUBLIC_IP="$candidate"
  if is_private_ip "$PUBLIC_IP"; then
    warn "the detected address $PUBLIC_IP is private - the invite code will contain it."
    warn "If this machine is behind NAT, re-run with W1NWIRE_PUBLIC_HOST=<public ip>."
  fi
  info "public address: $PUBLIC_IP"
}

# --- Configuration -----------------------------------------------------------
json_get() {
  python3 -c 'import json,sys
print(json.load(open(sys.argv[1])).get(sys.argv[2], ""))' "$1" "$2"
}

cfg_peer_count() {
  python3 -c 'import json,sys; print(len(json.load(open(sys.argv[1])).get("peers") or []))' "$CONF_PATH"
}

validate_config() {
  python3 -c 'import json,sys; json.load(open(sys.argv[1]))' "$CONF_PATH" 2> /dev/null \
    || die "$CONF_PATH is not valid JSON - fix or remove it and re-run"
}

ensure_directories() {
  install -d -m 0700 -o root -g root "$CONF_DIR"
  install -d -m 0700 -o root -g root "$INVITE_DIR"
}

ensure_keys() {
  if [ -s "$KEY_PATH" ]; then
    info "reusing the server key pair in $KEY_PATH"
  else
    step "generating the server key pair"
    "$BIN_PATH" keygen -out "$KEY_PATH" > /dev/null
    info "wrote $KEY_PATH (0600)"
  fi
  chmod 0600 "$KEY_PATH"
  SERVER_PUBLIC_KEY="$(json_get "$KEY_PATH" public_key)"
  SERVER_PRIVATE_KEY="$(json_get "$KEY_PATH" private_key)"
  if [ -z "$SERVER_PUBLIC_KEY" ] || [ -z "$SERVER_PRIVATE_KEY" ]; then
    die "$KEY_PATH does not contain a key pair"
  fi
}

write_default_config() {
  python3 - "$CONF_PATH" "$PORT" "$SERVER_PRIVATE_KEY" "$TUN_NAME" "$TUN_SERVER_ADDRESS" "$TUN_MTU" <<'PY'
import json
import sys

path, port, key, tun_name, tun_address, mtu = sys.argv[1:7]

config = {
    "listen": "0.0.0.0:%s" % port,
    "server_private_key": key,
    "tun": {
        "enabled": True,
        "name": tun_name,
        "address": tun_address,
        "mtu": int(mtu),
    },
    "nat": {
        "enabled": True,
        "outbound_interface": "",
    },
    "http": {
        "metrics": {
            "enabled": True,
            "host": "127.0.0.1",
            "port": 9187,
        }
    },
    "peers": [],
}

with open(path, "w", encoding="utf-8") as handle:
    json.dump(config, handle, indent=2)
    handle.write("\n")
PY
  chmod 0600 "$CONF_PATH"
}

patch_listen_port() {
  python3 - "$CONF_PATH" "$PORT" <<'PY'
import json
import sys

path, port = sys.argv[1], sys.argv[2]

with open(path, encoding="utf-8") as handle:
    config = json.load(handle)

config["listen"] = "0.0.0.0:%s" % port

with open(path, "w", encoding="utf-8") as handle:
    json.dump(config, handle, indent=2)
    handle.write("\n")
PY
  chmod 0600 "$CONF_PATH"
}

ensure_config() {
  if [ -s "$CONF_PATH" ]; then
    validate_config
    info "reusing the existing $CONF_PATH"
    local listen
    listen=$(json_get "$CONF_PATH" listen)
    if [ "$PORT_SET" -eq 1 ] && [ "$listen" != "0.0.0.0:$PORT" ]; then
      step "updating the listen address to 0.0.0.0:$PORT"
      patch_listen_port
      RESTART_NEEDED=1
    fi
    local nat
    nat=$(python3 -c 'import json,sys; print(str(json.load(open(sys.argv[1])).get("nat",{}).get("enabled",False)).lower())' "$CONF_PATH")
    if [ "$nat" = "false" ]; then
      info "note: nat.enabled is false in $CONF_PATH - the installer still manages the MASQUERADE rule"
    fi
  else
    step "writing $CONF_PATH"
    write_default_config
    info "wrote $CONF_PATH (0600)"
  fi
}

# --- Peers -------------------------------------------------------------------
save_invite() {
  local name="$1" code="$2"
  printf '%s\n' "$code" > "$INVITE_DIR/$name.invite"
  chmod 0600 "$INVITE_DIR/$name.invite"
}

load_saved_invite() {
  local name="$1"
  if [ -s "$INVITE_DIR/$name.invite" ]; then
    cat "$INVITE_DIR/$name.invite"
  fi
}

# Fallback used only when the binary has no `peer add` sub-command yet: create
# a client key pair, append the peer to server.json and build the invite code
# with the exact format the desktop app expects.
create_peer_fallback() {
  local name="$1" client_key="$WORKDIR/client.key" invite

  "$BIN_PATH" keygen -out "$client_key" > /dev/null
  chmod 0600 "$client_key"

  invite=$(python3 - "$CONF_PATH" "$client_key" "$name" "$(peer_host)" "$SERVER_PUBLIC_KEY" "$TUN_SUBNET" "$TUN_SERVER_IP" <<'PY'
import base64
import json
import sys

conf_path, key_path, name, endpoint, server_pub, subnet, gateway = sys.argv[1:8]

with open(conf_path, encoding="utf-8") as handle:
    config = json.load(handle)
with open(key_path, encoding="utf-8") as handle:
    client = json.load(handle)

used = {peer.get("tunnel_address") for peer in config.get("peers") or []}
address = None
for last in range(2, 255):
    candidate = "10.66.0.%d" % last
    if candidate not in used:
        address = candidate
        break
if address is None:
    raise SystemExit("no free tunnel address left in 10.66.0.0/24")

config.setdefault("peers", []).append({
    "id": name,
    "public_key": client["public_key"],
    "tunnel_address": address,
    "allowed_ips": [subnet, address + "/32"],
})

with open(conf_path, "w", encoding="utf-8") as handle:
    json.dump(config, handle, indent=2)
    handle.write("\n")

payload = {
    "v": 1,
    "n": name,
    "h": endpoint,
    "k": server_pub,
    "p": client["private_key"],
    "a": address,
    "g": gateway,
}
compact = json.dumps(payload, separators=(",", ":"), sort_keys=True).encode()
print("ww1." + base64.urlsafe_b64encode(compact).decode().rstrip("="))
PY
  )
  rm -f -- "$client_key"
  chmod 0600 "$CONF_PATH"
  printf '%s\n' "$invite"
}

# Public endpoint written into an invite code. W1NWIRE_PUBLIC_HOST may already
# carry a port, in which case it is used as given.
peer_host() {
  case "$PUBLIC_IP" in
    *:[0-9]*) printf '%s' "$PUBLIC_IP" ;;
    *) printf '%s:%s' "$PUBLIC_IP" "$PORT" ;;
  esac
}

create_peer() {
  local name="$1" output="" rc=0 before

  INVITE=""
  before=$(cfg_peer_count)

  # `peer add` is the supported path and writes the peer into server.json.
  # Current builds require the reachable endpoint (-host) because they refuse
  # to mint an invite for 0.0.0.0; older builds reject the unknown flag. The
  # second form is only tried when the first one failed *and* wrote nothing,
  # so a partially successful call can never add the peer twice.
  set +e
  output=$("$BIN_PATH" peer add -config "$CONF_PATH" -name "$name" -host "$(peer_host)" 2>&1)
  rc=$?
  set -e

  if [ "$rc" -ne 0 ] && [ "$(cfg_peer_count)" -eq "$before" ]; then
    set +e
    output=$("$BIN_PATH" peer add -config "$CONF_PATH" -name "$name" 2>&1)
    rc=$?
    set -e
  fi

  if [ "$rc" -eq 0 ]; then
    INVITE=$(printf '%s\n' "$output" | grep -m1 '^ww1\.' || true)
    if [ -n "$INVITE" ]; then
      return 0
    fi
    warn "peer add succeeded but printed no invite code"
  else
    warn "peer add is not available in this build:"
    printf '%s\n' "$output" | tail -n 3 | sed 's/^/      /' >&2
  fi

  # If the peer was written anyway, do not add a second one.
  if [ "$(cfg_peer_count)" -gt 0 ]; then
    INVITE=$(printf '%s\n' "$output" | grep -m1 'ww1\.' || true)
    return 0
  fi

  info "using the installer's built-in peer provisioning instead"
  INVITE=$(create_peer_fallback "$name" | tail -n 1)
  [ -n "$INVITE" ] || return 1
  return 0
}

ensure_first_peer() {
  local count

  count=$(cfg_peer_count)
  if [ "$count" -gt 0 ]; then
    info "the configuration already has $count peer(s)"
    INVITE=$(load_saved_invite "$PEER_NAME")
    return 0
  fi

  # The server refuses to start with an empty peer list, so the first peer has
  # to exist before the unit is started for the first time.
  step "creating the first peer ('$PEER_NAME')"
  if ! create_peer "$PEER_NAME"; then
    die "the first peer could not be created; the server cannot start without one"
  fi
  if [ -n "$INVITE" ]; then
    save_invite "$PEER_NAME" "$INVITE"
    info "invite code saved to $INVITE_DIR/$PEER_NAME.invite (0600)"
  fi
}

# --- systemd -----------------------------------------------------------------
install_unit() {
  local source_file="$SCRIPT_DIR/systemd/w1nwire-server.service"

  step "installing the systemd unit"
  if [ -f "$source_file" ]; then
    install -m 0644 -o root -g root "$source_file" "$UNIT_PATH"
  else
    cat > "$UNIT_PATH" <<'UNIT_EOF'
# =============================================================================
# W1nWire — systemd unit for the VPN server.
#
# Installed to /etc/systemd/system/w1nwire-server.service by deploy/install.sh.
# Keep this file and the copy embedded in deploy/install.sh in sync.
#
# WHY ROOT AND NOT User=w1nwire
# -----------------------------
# The daemon does two things that need privileges:
#
#   1. it opens /dev/net/tun and configures the interface (CAP_NET_ADMIN),
#   2. it shells out to `ip`, `iptables` and `sysctl` (internal/tun) so the
#      masquerading and forwarding rules exist even if the operator never ran
#      install.sh.
#
# Running it as an unprivileged user would need an udev rule for /dev/net/tun,
# write access to /run/xtables.lock, and CAP_NET_ADMIN in the ambient set -
# while `sysctl -w net.ipv4.ip_forward=1` needs write access to /proc/sys,
# which the hardening options below would have to re-open anyway. Root with a
# frozen filesystem, no new privileges, no home directory, no kernel module
# access and a private /tmp is the smaller, more auditable surface. The binary
# is a static Go program whose only input is /etc/w1nwire/server.json.
# =============================================================================

[Unit]
Description=W1nWire VPN server
Documentation=https://github.com/w1nchi/w1nwire
After=network-online.target
Wants=network-online.target

# Never start without a configuration: a missing or half-written server.json
# must not turn into a restart loop.
ConditionPathExists=/etc/w1nwire/server.json

# Stop restarting after five failures in five minutes instead of hammering.
StartLimitIntervalSec=300
StartLimitBurst=5

[Service]
Type=simple
User=root
Group=root

# The configuration path is the only argument; everything else is inside the
# JSON file (listen address, TUN, NAT, metrics, peers).
ExecStart=/usr/local/bin/w1nwire-server -config /etc/w1nwire/server.json

# A VPN server must come back by itself. SIGTERM lets the process close the
# UDP socket and the TUN device; there is no SIGHUP reload, use
# `systemctl restart w1nwire-server` after editing server.json.
Restart=always
RestartSec=5s
TimeoutStopSec=20s
KillSignal=SIGTERM

# Logs go to the journal only - no file to rotate, no /var/log to manage.
StandardOutput=journal
StandardError=journal
SyslogIdentifier=w1nwire-server

# --- Hardening ---------------------------------------------------------------
# The process can allocate executable memory only through its own text segment;
# it never drops privileges, never creates namespaces and never touches the
# kernel below /proc/sys (except when it re-applies the forwarding sysctl).
NoNewPrivileges=yes

# /usr, /boot and /etc are read only for the service. The configuration is
# written by `w1nwire-server peer add` (run by the operator, not by the unit),
# so the service itself never needs to write there.
ProtectSystem=full
ProtectHome=yes
PrivateTmp=yes

# No module loading, no control-group games, no clock or hostname changes.
ProtectKernelModules=yes
ProtectControlGroups=yes
ProtectClock=yes
ProtectHostname=yes
ProtectProc=invisible

RestrictSUIDSGID=yes
RestrictRealtime=yes
RestrictNamespaces=yes
LockPersonality=yes
SystemCallArchitectures=native

# Plain UDP plus AF_NETLINK for the interface configuration, AF_UNIX for
# journald. Nothing else is needed.
RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX AF_NETLINK

# CAP_NET_ADMIN  create/configure the TUN device, run iptables, write the
#                forwarding sysctl
# CAP_NET_BIND_SERVICE  unlikely to be needed (UDP 51820 is unprivileged) but
#                harmless and keeps a future privileged port working
# CAP_DAC_OVERRIDE  keep reading root-only files regardless of their mode
CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_RAW CAP_NET_BIND_SERVICE CAP_DAC_OVERRIDE
AmbientCapabilities=CAP_NET_ADMIN CAP_NET_RAW

# ProtectKernelTunables is deliberately NOT set: at startup the server applies
# `sysctl -w net.ipv4.ip_forward=1` through internal/tun, and a read-only
# /proc/sys would make that call fail (the server only logs a warning, but the
# forwarding would silently stay off on a fresh boot).

# --- Limits ------------------------------------------------------------------
LimitNOFILE=1048576
TasksMax=512

[Install]
WantedBy=multi-user.target
UNIT_EOF
    chmod 0644 "$UNIT_PATH"
  fi
  info "wrote $UNIT_PATH"
}

wait_for_active() {
  local unit="$1" tries=0
  while [ "$tries" -lt 40 ]; do
    if systemctl is-active --quiet "$unit"; then
      return 0
    fi
    tries=$((tries + 1))
    sleep 0.5
  done
  return 1
}

show_failure() {
  local unit="$1"
  error "$unit did not become active"
  printf '\n' >&2
  systemctl status --no-pager --lines=15 "$unit" >&2 || true
  printf '\n' >&2
  journalctl --no-pager -n 40 -u "$unit" >&2 || true
}

start_service() {
  step "starting the service"
  systemctl daemon-reload
  systemctl enable --quiet w1nwire-server.service
  systemctl reset-failed w1nwire-server.service 2> /dev/null || true
  systemctl restart w1nwire-server.service

  if ! wait_for_active w1nwire-server.service; then
    show_failure w1nwire-server.service
    die "the server did not start (the details above are the reason)"
  fi
  info "w1nwire-server is active"
}

check_metrics() {
  local body
  if ! have curl; then
    return 0
  fi
  body=$(curl -fsS --max-time 3 http://127.0.0.1:9187/healthz 2> /dev/null || true)
  if [ "$body" = "ok" ]; then
    info "metrics endpoint: http://127.0.0.1:9187/healthz -> ok"
  else
    info "metrics endpoint not reachable on 127.0.0.1:9187 (metrics may be disabled)"
  fi
}

# --- Networking --------------------------------------------------------------
detect_wan_if() {
  if [ -n "$WAN_IF" ]; then
    return 0
  fi
  if [ -n "${W1NWIRE_WAN_IF:-}" ]; then
    WAN_IF="$W1NWIRE_WAN_IF"
    info "outbound interface (from W1NWIRE_WAN_IF): $WAN_IF"
    return 0
  fi
  WAN_IF=$(ip -4 route show default 2> /dev/null | awk '{ for (i = 1; i < NF; i++) if ($i == "dev") { print $(i + 1); exit } }')
  if [ -z "$WAN_IF" ]; then
    die "the default outbound interface could not be detected; set W1NWIRE_WAN_IF=<iface>"
  fi
  info "outbound interface: $WAN_IF"
}

enable_forwarding() {
  step "enabling IPv4 forwarding"
  cat > "$SYSCTL_PATH" <<'SYSCTL_EOF'
# W1nWire: the tunnel subnet 10.66.0.0/24 has to be forwarded to the internet,
# otherwise the server accepts the tunnel but nothing leaves the machine.
# Installed by deploy/install.sh, removed by `install.sh --uninstall`.
net.ipv4.ip_forward = 1
SYSCTL_EOF
  chmod 0644 "$SYSCTL_PATH"
  sysctl -q -w net.ipv4.ip_forward=1 > /dev/null
  info "net.ipv4.ip_forward=1 (persistent via $SYSCTL_PATH)"
}

install_nat_rules() {
  step "installing the NAT and forwarding rules"

  if ! iptables -w 5 -t nat -C POSTROUTING -s "$TUN_SUBNET" -o "$WAN_IF" -j MASQUERADE 2> /dev/null; then
    iptables -w 5 -t nat -A POSTROUTING -s "$TUN_SUBNET" -o "$WAN_IF" -j MASQUERADE \
      || die "could not add the MASQUERADE rule for $TUN_SUBNET on $WAN_IF"
    info "nat POSTROUTING: MASQUERADE $TUN_SUBNET via $WAN_IF"
  else
    info "nat POSTROUTING: MASQUERADE already present"
  fi

  if ! iptables -w 5 -C FORWARD -i "$TUN_NAME" -o "$WAN_IF" -j ACCEPT 2> /dev/null; then
    iptables -w 5 -I FORWARD 1 -i "$TUN_NAME" -o "$WAN_IF" -j ACCEPT \
      || die "could not add the FORWARD rule for $TUN_NAME"
    info "filter FORWARD: accept $TUN_NAME -> $WAN_IF"
  else
    info "filter FORWARD: $TUN_NAME -> $WAN_IF already accepted"
  fi

  if ! iptables -w 5 -C FORWARD -i "$WAN_IF" -o "$TUN_NAME" -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT 2> /dev/null; then
    iptables -w 5 -I FORWARD 1 -i "$WAN_IF" -o "$TUN_NAME" -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT \
      || warn "could not add the return-traffic FORWARD rule (conntrack module missing?)"
    info "filter FORWARD: accept established traffic $WAN_IF -> $TUN_NAME"
  else
    info "filter FORWARD: return traffic already accepted"
  fi

  # /run/xtables.lock must exist for the daemon's own iptables calls; the
  # service runs as root, so this is only informational.
  info "rules are re-applied by the server on start (nat.enabled=true)"
}

open_udp_port() {
  local port="$1" ufw_active=0 oracle_reject=0

  if [ "$FIREWALL" -ne 1 ]; then
    info "firewall: skipped UDP/$port (--no-firewall)"
    return 0
  fi

  if have ufw && ufw status 2> /dev/null | grep -q '^Status: active'; then
    ufw_active=1
    if ufw allow "$port/udp" > /dev/null 2>&1; then
      info "ufw: allowed UDP/$port"
    else
      warn "ufw: could not add the rule for UDP/$port"
    fi
  fi

  # Oracle's Ubuntu images ship a ruleset that ends with
  # "-A INPUT -j REJECT", so everything except SSH is blocked by default.
  # An ACCEPT inserted at the top of INPUT is what makes the VPN port
  # reachable. It is inserted even when ufw is active: on an image whose saved
  # ruleset ends in a REJECT, the raw rule guarantees the port is reachable
  # regardless of how ufw and the persisted ruleset are ordered.
  if iptables -w 5 -S INPUT 2> /dev/null | grep -q -- '-j REJECT'; then
    oracle_reject=1
  fi

  if [ "$ufw_active" -eq 0 ] || [ "$oracle_reject" -eq 1 ]; then
    if iptables -w 5 -C INPUT -p udp --dport "$port" -j ACCEPT 2> /dev/null; then
      info "iptables: INPUT already accepts UDP/$port"
    else
      iptables -w 5 -I INPUT 1 -p udp --dport "$port" -j ACCEPT \
        || warn "iptables: could not add the INPUT rule for UDP/$port"
      info "iptables: inserted an INPUT ACCEPT rule for UDP/$port"
    fi
  fi
}

persist_rules() {
  step "persisting the iptables rules"
  if have netfilter-persistent; then
    netfilter-persistent save > /dev/null 2>&1 || warn "netfilter-persistent save failed"
    info "rules saved with netfilter-persistent ($IPTABLES_RULES)"
    return 0
  fi

  if have apt-get; then
    # Non-interactive: keep the existing /etc/iptables/rules.v4 (Oracle ships
    # one) and never ask about saving the current ruleset.
    if ! dpkg -s iptables-persistent > /dev/null 2>&1; then
      apt_update_once
      printf '%s\n' \
        'iptables-persistent iptables-persistent/autosave_v4 boolean true' \
        'iptables-persistent iptables-persistent/autosave_v6 boolean true' \
        | debconf-set-selections 2> /dev/null || true
      DEBIAN_FRONTEND=noninteractive NEEDRESTART_MODE=a \
        apt-get install -y -qq --no-install-recommends \
        -o Dpkg::Options::=--force-confdef -o Dpkg::Options::=--force-confold \
        iptables-persistent > /dev/null 2>&1 || warn "iptables-persistent could not be installed"
    fi
    if have netfilter-persistent; then
      netfilter-persistent save > /dev/null 2>&1 || warn "netfilter-persistent save failed"
      info "rules saved with netfilter-persistent ($IPTABLES_RULES)"
      return 0
    fi
  fi

  if have iptables-save; then
    install -d -m 0755 /etc/iptables
    iptables-save > "$IPTABLES_RULES"
    chmod 0600 "$IPTABLES_RULES"
    info "rules written to $IPTABLES_RULES"
    warn "iptables-persistent is not installed, so $IPTABLES_RULES is not restored automatically."
    warn "Install it with: apt-get install -y iptables-persistent"
  else
    warn "iptables-save is missing; the NAT rules will be lost on reboot"
  fi
}

# --- Keepalive ---------------------------------------------------------------
is_oracle_instance() {
  local tag="" product=""

  if [ -r /sys/class/dmi/id/chassis_asset_tag ]; then
    tag=$(tr -d '\n' < /sys/class/dmi/id/chassis_asset_tag 2> /dev/null || true)
  fi
  case "$tag" in
    *OracleCloud*) return 0 ;;
  esac

  if [ -r /sys/class/dmi/id/product_name ]; then
    product=$(tr -d '\n' < /sys/class/dmi/id/product_name 2> /dev/null || true)
  fi
  case "$product" in
    *OracleCloud* | *Oracle*) return 0 ;;
  esac

  if have dmidecode; then
    product=$(dmidecode -s system-product-name 2> /dev/null || true)
    case "$product" in
      *Oracle*) return 0 ;;
    esac
  fi

  return 1
}

write_keepalive_units() {
  cat > "$KEEPALIVE_UNIT_PATH" <<'UNIT_EOF'
[Unit]
Description=W1nWire idle keepalive (Oracle Cloud Always Free)
Documentation=https://github.com/w1nchi/w1nwire

[Service]
Type=oneshot
ExecStart=/usr/local/bin/w1nwire-keepalive.sh
# The VPN always wins: the keepalive only has to show up in the utilization
# metrics, it must never delay a packet.
Nice=19
IOSchedulingClass=idle
UNIT_EOF
  chmod 0644 "$KEEPALIVE_UNIT_PATH"

  cat > "$KEEPALIVE_TIMER_PATH" <<'TIMER_EOF'
[Unit]
Description=Run the W1nWire idle keepalive every five minutes

[Timer]
OnBootSec=3min
OnUnitActiveSec=5min
AccuracySec=30s
Persistent=true
Unit=w1nwire-keepalive.service

[Install]
WantedBy=timers.target
TIMER_EOF
  chmod 0644 "$KEEPALIVE_TIMER_PATH"
}

install_keepalive() {
  local source_file="$SCRIPT_DIR/scripts/oracle-idle-keepalive.sh"

  if [ "$KEEPALIVE" -ne 1 ]; then
    step "skipping the idle keepalive (--no-keepalive)"
    return 0
  fi

  step "installing the Oracle idle keepalive"
  if is_oracle_instance; then
    info "Oracle Cloud instance detected (DMI chassis asset tag / product name)"
  else
    warn "this does not look like an Oracle Cloud instance; installing the keepalive anyway"
    warn "it costs a few seconds of CPU every five minutes; remove it with"
    warn "  systemctl disable --now w1nwire-keepalive.timer && rm -f $KEEPALIVE_BIN_PATH"
  fi

  if [ -f "$source_file" ]; then
    install -m 0755 -o root -g root "$source_file" "$KEEPALIVE_BIN_PATH"
  else
    cat > "$KEEPALIVE_BIN_PATH" <<'KEEPALIVE_EOF'
#!/usr/bin/env bash
# =============================================================================
# W1nWire — Oracle Cloud idle keepalive.
#
# WHY: an Oracle Cloud Always Free instance can be reclaimed when its CPU,
# network and memory utilization stay below the thresholds for seven days. A
# VPN exit node that nobody uses sits at almost zero on all three metrics, so
# a free instance can be stopped even though nothing is wrong with it.
#
# This job is a cheap floor under those metrics: every five minutes it burns
# CPU for a few seconds (dd from /dev/zero to /dev/null), allocates a few MiB
# of RAM (a file in /dev/shm) and moves a little traffic (one HTTPS request).
# It is small on purpose: it must not cost money and must not compete with the
# VPN for CPU (Nice=19 in the unit).
#
# HONEST LIMITATION: the published rule is about utilization over a week, so a
# short periodic spike cannot make an unused instance look busy - it only
# keeps the instance out of the "nothing happened at all" bucket. The real
# defense is to actually use the VPN. See docs/ORACLE.md.
#
# Installed by deploy/install.sh as /usr/local/bin/w1nwire-keepalive.sh and run
# by w1nwire-keepalive.timer. The canonical copy is
# deploy/scripts/oracle-idle-keepalive.sh in the repository.
#
# Tunables: W1NWIRE_KEEPALIVE_CPU_SECONDS (20), W1NWIRE_KEEPALIVE_MEM_MIB (16),
# W1NWIRE_KEEPALIVE_NET_BYTES (262144), or /etc/default/w1nwire-keepalive.
# =============================================================================
set -euo pipefail

CPU_SECONDS="${W1NWIRE_KEEPALIVE_CPU_SECONDS:-20}"
MEM_MIB="${W1NWIRE_KEEPALIVE_MEM_MIB:-16}"
NET_BYTES="${W1NWIRE_KEEPALIVE_NET_BYTES:-262144}"
DRY_RUN=0

KEEPALIVE_URLS=(
  "https://speed.cloudflare.com/__down?bytes=${NET_BYTES}"
  "https://cloudflare.com/cdn-cgi/trace"
  "https://connectivitycheck.gstatic.com/generate_204"
)

log() {
  printf '%s w1nwire-keepalive: %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*"
}

usage() {
  cat <<'USAGE'
w1nwire-keepalive - keep an Oracle Cloud Always Free instance out of the idle bucket

Usage:
  w1nwire-keepalive.sh            run one keepalive cycle
  w1nwire-keepalive.sh --dry-run  print what a cycle would do, change nothing
  w1nwire-keepalive.sh --help     this text

Tunables (environment, or /etc/default/w1nwire-keepalive):
  W1NWIRE_KEEPALIVE_CPU_SECONDS   seconds of CPU load per cycle   (default 20)
  W1NWIRE_KEEPALIVE_MEM_MIB       MiB allocated in /dev/shm       (default 16)
  W1NWIRE_KEEPALIVE_NET_BYTES     bytes downloaded per cycle      (default 262144)
USAGE
}

burn_cpu() {
  case "$CPU_SECONDS" in
    '' | *[!0-9]*)
      log "WARNING: invalid W1NWIRE_KEEPALIVE_CPU_SECONDS='$CPU_SECONDS', skipping the CPU load"
      return 0
      ;;
  esac
  case "$CPU_SECONDS" in 0) return 0 ;; esac
  if ! command -v dd > /dev/null 2>&1; then
    log "WARNING: dd is not installed, skipping the CPU load"
    return 0
  fi
  if [ "$DRY_RUN" -eq 1 ]; then
    log "dry-run: would burn CPU for ${CPU_SECONDS}s"
    return 0
  fi
  local deadline=$((SECONDS + CPU_SECONDS)) rounds=0
  while [ "$SECONDS" -lt "$deadline" ]; do
    dd if=/dev/zero of=/dev/null bs=1M count=32 status=none || true
    rounds=$((rounds + 1))
  done
  log "CPU load: ${CPU_SECONDS}s (${rounds} dd rounds)"
}

touch_memory() {
  case "$MEM_MIB" in
    '' | *[!0-9]*)
      log "WARNING: invalid W1NWIRE_KEEPALIVE_MEM_MIB='$MEM_MIB', skipping the memory touch"
      return 0
      ;;
  esac
  case "$MEM_MIB" in 0) return 0 ;; esac
  if [ ! -d /dev/shm ] || [ ! -w /dev/shm ]; then
    log "WARNING: /dev/shm is not writable, skipping the memory touch"
    return 0
  fi
  if ! command -v dd > /dev/null 2>&1; then
    log "WARNING: dd is not installed, skipping the memory touch"
    return 0
  fi
  if [ "$DRY_RUN" -eq 1 ]; then
    log "dry-run: would allocate ${MEM_MIB} MiB in /dev/shm"
    return 0
  fi
  local file="/dev/shm/.w1nwire-keepalive.$$"
  if dd if=/dev/zero of="$file" bs=1M count="$MEM_MIB" status=none 2> /dev/null; then
    log "memory touch: ${MEM_MIB} MiB"
  else
    log "WARNING: the memory touch failed (tmpfs full?)"
  fi
  rm -f -- "$file"
}

touch_network() {
  case "$NET_BYTES" in
    '' | *[!0-9]*)
      log "WARNING: invalid W1NWIRE_KEEPALIVE_NET_BYTES='$NET_BYTES', skipping the network touch"
      return 0
      ;;
  esac
  case "$NET_BYTES" in 0) return 0 ;; esac
  if [ "$DRY_RUN" -eq 1 ]; then
    log "dry-run: would download about ${NET_BYTES} bytes"
    return 0
  fi
  local url
  if command -v curl > /dev/null 2>&1; then
    for url in "${KEEPALIVE_URLS[@]}"; do
      if curl -fsSL --max-time 25 --retry 1 -o /dev/null "$url" 2> /dev/null; then
        log "network touch: ${url}"
        return 0
      fi
    done
  elif command -v wget > /dev/null 2>&1; then
    for url in "${KEEPALIVE_URLS[@]}"; do
      if wget -q -T 25 -O /dev/null "$url" 2> /dev/null; then
        log "network touch: ${url}"
        return 0
      fi
    done
  else
    log "WARNING: neither curl nor wget is installed, skipping the network touch"
    return 0
  fi
  if command -v getent > /dev/null 2>&1; then
    getent hosts one.one.one.one > /dev/null 2>&1 || true
  fi
  log "WARNING: no keepalive endpoint was reachable"
  return 0
}

main() {
  case "${1:-}" in
    --help | -h)
      usage
      return 0
      ;;
    --dry-run)
      DRY_RUN=1
      ;;
    '')
      ;;
    *)
      printf 'unknown option: %s (try --help)\n' "$1" >&2
      return 2
      ;;
  esac

  # Optional tunables, see the header.
  if [ -r /etc/default/w1nwire-keepalive ]; then
    # shellcheck disable=SC1091
    . /etc/default/w1nwire-keepalive
  fi

  log "starting a keepalive cycle"
  burn_cpu
  touch_memory
  touch_network
  log "keepalive cycle finished"
}

main "$@"
KEEPALIVE_EOF
    chmod 0755 "$KEEPALIVE_BIN_PATH"
  fi

  write_keepalive_units
  systemctl daemon-reload
  systemctl enable --quiet w1nwire-keepalive.timer
  systemctl restart w1nwire-keepalive.timer
  if systemctl is-active --quiet w1nwire-keepalive.timer; then
    info "w1nwire-keepalive.timer is active (every 5 minutes)"
  else
    warn "w1nwire-keepalive.timer is not active; check: systemctl status w1nwire-keepalive.timer"
  fi
}

# --- iPhone (WireGuard) ------------------------------------------------------
ensure_wireguard_packages() {
  local missing=()

  have wg || missing+=(wireguard)
  have wg-quick || missing+=(wireguard-tools)
  have qrencode || missing+=(qrencode)

  if [ "${#missing[@]}" -eq 0 ]; then
    info "wireguard and qrencode are already installed"
    return 0
  fi
  step "installing ${missing[*]}"
  apt_update_once
  apt_install "${missing[@]}" || die "could not install wireguard/qrencode"
  have wg || die "wg is still missing after the installation"
  have qrencode || die "qrencode is still missing after the installation"
}

wg_server_public_key() {
  if [ -s /etc/wireguard/wg0-server.pub ]; then
    cat /etc/wireguard/wg0-server.pub
    return 0
  fi
  if have wg && wg show "$WG_IFACE" public-key 2> /dev/null; then
    return 0
  fi
  return 1
}

new_wg_keypair() {
  local priv
  priv=$(wg genkey)
  printf '%s\n' "$priv"
  printf '%s\n' "$priv" | wg pubkey
}

wg_client_conf_path() {
  local n="$1"
  printf '/etc/wireguard/wg0-client-%s.conf' "$n"
}

write_wg_client_conf() {
  local number="$1" address="$2" private_key="$3" server_pub="$4" path
  path="$(wg_client_conf_path "$number")"

  (
    umask 077
    cat > "$path" <<CLIENT_EOF
# W1nWire / WireGuard client configuration for phone $number.
# One file per phone. Treat it like a password: it is the credentials of that
# device. Scan it with the WireGuard app or import it as a file.
[Interface]
PrivateKey = ${private_key}
Address = ${address}/32
DNS = 1.1.1.1
MTU = 1420

[Peer]
PublicKey = ${server_pub}
Endpoint = ${PUBLIC_IP}:${WG_PORT}
AllowedIPs = 0.0.0.0/0
PersistentKeepalive = 25
CLIENT_EOF
  )
  chmod 0600 "$path"
  printf '%s\n' "$path"
}

append_wg_peer() {
  local public_key="$1" address="$2" number="$3"

  (
    umask 077
    cat >> /etc/wireguard/wg0.conf <<PEER_EOF

[Peer]
# phone-${number}
PublicKey = ${public_key}
AllowedIPs = ${address}/32
PEER_EOF
  )
}

print_qr() {
  local path="$1"
  if ! have qrencode; then
    warn "qrencode is not installed; the configuration is in $path"
    return 0
  fi
  if [ ! -s "$path" ]; then
    warn "no client configuration at $path"
    return 0
  fi
  printf '\n'
  qrencode -t ansiutf8 < "$path" || warn "the QR code could not be rendered in this terminal"
  printf '\n'
}

setup_ios() {
  local wg_conf=/etc/wireguard/wg0.conf
  local server_priv="" server_pub="" phone_priv="" phone_pub="" phone_path="" client_path

  step "setting up WireGuard for the iPhone (UDP $WG_PORT)"
  ensure_wireguard_packages
  detect_wan_if

  if [ -f "$wg_conf" ]; then
    info "keeping the existing $wg_conf (it holds the phone keys)"
    systemctl enable --quiet wg-quick@"$WG_IFACE".service 2> /dev/null || true
    systemctl start wg-quick@"$WG_IFACE".service 2> /dev/null || warn "wg-quick@$WG_IFACE could not be started"
    open_udp_port "$WG_PORT"
    persist_rules

    client_path=$(compgen -G "/etc/wireguard/wg0-client-*.conf" 2> /dev/null | sort -V | tail -n 1 || true)
    if [ -z "$client_path" ]; then
      warn "wg0.conf exists but no phone configuration was found; adding a new phone"
      add_phone_peer
      return 0
    fi
    info "client configuration: $client_path"
    print_qr "$client_path"
    printf 'Scan this with the free WireGuard app from the App Store.\n'
    return 0
  fi

  install -d -m 0700 /etc/wireguard

  server_priv=$(wg genkey)
  server_pub=$(printf '%s\n' "$server_priv" | wg pubkey)
  phone_priv=$(wg genkey)
  phone_pub=$(printf '%s\n' "$phone_priv" | wg pubkey)

  (
    umask 077
    cat > "$wg_conf" <<WG_EOF
# W1nWire — WireGuard endpoint for the iPhone.
#
# WHY THIS EXISTS
# ---------------
# The native W1nWire protocol needs a VPN network extension on iOS, and Apple
# only grants that entitlement to paid Apple Developer accounts. WireGuard is
# the free way around it: the official app is free in the App Store and the
# WireGuard endpoint below is what it connects to. From this point the phone
# speaks WireGuard, not W1nWire.
#
# Installed by deploy/install.sh --ios.
# Removed by: bash install.sh --ios --uninstall
[Interface]
Address = ${WG_ADDRESS}
ListenPort = ${WG_PORT}
PrivateKey = ${server_priv}
MTU = 1420

# Forward and masquerade the phone traffic. wg-quick re-runs these on every
# start, so they survive a reboot. %i is substituted with the interface name.
PostUp = sysctl -q -w net.ipv4.ip_forward=1
PostUp = iptables -C FORWARD -i %i -j ACCEPT 2>/dev/null || iptables -I FORWARD 1 -i %i -j ACCEPT
PostUp = iptables -C FORWARD -o %i -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT 2>/dev/null || iptables -I FORWARD 1 -o %i -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT
PostUp = iptables -t nat -C POSTROUTING -s ${WG_SUBNET} -o ${WAN_IF} -j MASQUERADE 2>/dev/null || iptables -t nat -A POSTROUTING -s ${WG_SUBNET} -o ${WAN_IF} -j MASQUERADE
PostDown = iptables -D FORWARD -i %i -j ACCEPT 2>/dev/null || true
PostDown = iptables -D FORWARD -o %i -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT 2>/dev/null || true
PostDown = iptables -t nat -D POSTROUTING -s ${WG_SUBNET} -o ${WAN_IF} -j MASQUERADE 2>/dev/null || true

[Peer]
# phone-1
PublicKey = ${phone_pub}
AllowedIPs = 10.67.0.2/32
WG_EOF
  )
  chmod 0600 "$wg_conf"
  printf '%s\n' "$server_pub" > /etc/wireguard/wg0-server.pub
  chmod 0644 /etc/wireguard/wg0-server.pub
  info "wrote $wg_conf (0600)"

  phone_path=$(write_wg_client_conf 1 "10.67.0.2" "$phone_priv" "$server_pub" | tail -n 1)
  info "wrote $phone_path (0600)"

  systemctl enable --quiet wg-quick@"$WG_IFACE".service
  systemctl start wg-quick@"$WG_IFACE".service || warn "wg-quick@$WG_IFACE failed to start; check: journalctl -u wg-quick@$WG_IFACE"
  if systemctl is-active --quiet wg-quick@"$WG_IFACE".service; then
    info "wg-quick@$WG_IFACE is active"
  fi

  open_udp_port "$WG_PORT"
  persist_rules

  info "server: $WG_ADDRESS on UDP $WG_PORT, phone: 10.67.0.2"
  print_qr "$phone_path"
  printf 'Scan this with the free WireGuard app from the App Store, then enable the tunnel.\n'
  printf 'Check it in Safari: https://api.ipify.org should show %s.\n' "$PUBLIC_IP"
}

add_phone_peer() {
  local wg_conf=/etc/wireguard/wg0.conf
  local last=1 number=1 address private_key public_key server_pub path

  if [ ! -f "$wg_conf" ]; then
    die "no $wg_conf - run 'bash install.sh --ios' first"
  fi
  ensure_wireguard_packages

  last=$(grep -oE '10\.67\.0\.[0-9]+' "$wg_conf" 2> /dev/null | awk -F. '{print $4}' | sort -n | tail -n 1 || true)
  last=${last:-1}
  if [ "$last" -ge 254 ]; then
    die "no free address left in 10.67.0.0/24"
  fi
  address="10.67.0.$((last + 1))"
  number=$(grep -c '^\[Peer\]' "$wg_conf" 2> /dev/null || true)
  number=${number:-0}
  number=$((number + 1))

  server_pub=$(wg_server_public_key) || die "the WireGuard server public key could not be determined"
  private_key=$(wg genkey)
  public_key=$(printf '%s\n' "$private_key" | wg pubkey)

  step "adding phone-$number ($address) to $wg_conf"
  append_wg_peer "$public_key" "$address" "$number"
  path=$(write_wg_client_conf "$number" "$address" "$private_key" "$server_pub" | tail -n 1)
  info "wrote $path (0600)"

  systemctl restart wg-quick@"$WG_IFACE".service 2> /dev/null || warn "wg-quick@$WG_IFACE could not be restarted"
  print_qr "$path"
  printf 'Scan this with the free WireGuard app from the App Store.\n'
}

remove_ios() {
  local wg_conf=/etc/wireguard/wg0.conf

  step "removing the WireGuard for iPhone setup"
  systemctl disable --now wg-quick@"$WG_IFACE".service 2> /dev/null || true
  if [ -f "$wg_conf" ]; then
    rm -f -- "$wg_conf"
    info "removed $wg_conf"
  fi
  local f
  for f in /etc/wireguard/wg0-client-*.conf /etc/wireguard/wg0-server.pub; do
    if [ -f "$f" ]; then
      rm -f -- "$f"
      info "removed $f"
    fi
  done
  rmdir /etc/wireguard 2> /dev/null || true
  if [ "$FIREWALL" -eq 1 ]; then
    if have ufw; then
      ufw delete allow "$WG_PORT/udp" > /dev/null 2>&1 || true
    fi
    if have iptables; then
      if iptables -w 5 -C INPUT -p udp --dport "$WG_PORT" -j ACCEPT 2> /dev/null; then
        iptables -w 5 -D INPUT -p udp --dport "$WG_PORT" -j ACCEPT || true
        info "removed the INPUT rule for UDP/$WG_PORT"
      fi
      purge_rules_for '10\.67\.0\.0/24|wg0'
    fi
  fi
}

# --- Firewall cleanup --------------------------------------------------------
# Deletes every rule in the nat and filter tables that mentions the given
# pattern. `iptables -D` needs the exact specification, and the specification
# is exactly what `iptables-save` prints, so the saved line is replayed with
# -A replaced by -D.
purge_rules_for() {
  local pattern="$1" table line
  local -a spec

  for table in nat filter; do
    have iptables-save || return 0
    while IFS= read -r line; do
      [ -n "$line" ] || continue
      # shellcheck disable=SC2206 # the arguments are simple tokens by construction
      spec=(${line#-A })
      iptables -w 5 -t "$table" -D "${spec[@]}" 2> /dev/null || true
    done <<< "$(iptables-save -t "$table" 2> /dev/null | grep -E '^-A ' | grep -E -- "$pattern" || true)"
  done
}

remove_firewall_rules() {
  step "removing the firewall rules"
  if ! have iptables; then
    info "iptables is not installed, nothing to remove"
    return 0
  fi

  if [ "$FIREWALL" -eq 1 ]; then
    if have ufw; then
      ufw delete allow "$PORT/udp" > /dev/null 2>&1 || true
    fi
    if iptables -w 5 -C INPUT -p udp --dport "$PORT" -j ACCEPT 2> /dev/null; then
      iptables -w 5 -D INPUT -p udp --dport "$PORT" -j ACCEPT || true
      info "removed the INPUT rule for UDP/$PORT"
    fi
  fi

  purge_rules_for "${TUN_SUBNET//./\\.}|${TUN_NAME}"
  info "removed the NAT/forward rules for $TUN_SUBNET"

  if have netfilter-persistent; then
    netfilter-persistent save > /dev/null 2>&1 || true
  elif have iptables-save && [ -f "$IPTABLES_RULES" ]; then
    iptables-save > "$IPTABLES_RULES"
  fi
}

# --- Uninstall ---------------------------------------------------------------
do_uninstall() {
  step "uninstalling W1nWire"

  systemctl disable --now w1nwire-keepalive.timer 2> /dev/null || true
  systemctl disable --now w1nwire-server.service 2> /dev/null || true

  if [ -f "$UNIT_PATH" ]; then
    rm -f -- "$UNIT_PATH"
    info "removed $UNIT_PATH"
  fi
  if [ -f "$KEEPALIVE_UNIT_PATH" ]; then
    rm -f -- "$KEEPALIVE_UNIT_PATH"
    info "removed $KEEPALIVE_UNIT_PATH"
  fi
  if [ -f "$KEEPALIVE_TIMER_PATH" ]; then
    rm -f -- "$KEEPALIVE_TIMER_PATH"
    info "removed $KEEPALIVE_TIMER_PATH"
  fi
  systemctl daemon-reload 2> /dev/null || true
  systemctl reset-failed w1nwire-server.service 2> /dev/null || true

  if [ -f "$BIN_PATH" ]; then
    rm -f -- "$BIN_PATH"
    info "removed $BIN_PATH"
  fi
  if [ -f "$KEEPALIVE_BIN_PATH" ]; then
    rm -f -- "$KEEPALIVE_BIN_PATH"
    info "removed $KEEPALIVE_BIN_PATH"
  fi

  if [ -d "$CONF_DIR" ]; then
    rm -rf -- "$CONF_DIR"
    info "removed $CONF_DIR (keys, configuration and saved invite codes)"
  fi

  if [ -f "$SYSCTL_PATH" ]; then
    rm -f -- "$SYSCTL_PATH"
    info "removed $SYSCTL_PATH (net.ipv4.ip_forward keeps its current value until reboot)"
  fi

  remove_firewall_rules

  if [ "$WITH_IOS" -eq 1 ]; then
    remove_ios
  else
    if [ -f /etc/wireguard/wg0.conf ]; then
      info "leaving /etc/wireguard/wg0.conf in place (re-run with --ios --uninstall to remove it)"
    fi
  fi

  printf '\n'
  info "uninstall finished. The system is back to a plain VM: no service, no binary, no rules."
  info "The public IP and the cloud firewall rules (security list / NSG) were not touched."
}

# --- Summary -----------------------------------------------------------------
print_summary() {
  local address="${PUBLIC_IP}:${PORT}"

  printf '\n'
  if [ -z "$INVITE" ]; then
    INVITE=$(load_saved_invite "$PEER_NAME")
  fi

  printf '============================================================\n'
  printf ' W1nWire server is running\n'
  printf ' Address: %s\n' "$address"
  printf ' Invite code (copy the whole line into the W1nWire app):\n'
  if [ -n "$INVITE" ]; then
    printf '%s\n' "$INVITE"
  else
    printf ' (no invite code was generated - the configuration already had peers)\n'
  fi
  printf '============================================================\n'

  printf '\nNext:\n'
  if [ -z "$INVITE" ]; then
    printf '  Add a device:      sudo w1nwire-server peer add -config %s -name laptop -host %s\n' "$CONF_PATH" "$(peer_host)"
    printf '                     sudo systemctl restart w1nwire-server\n'
  fi
  printf '  Service status:    systemctl status w1nwire-server\n'
  printf '  Logs:              journalctl -u w1nwire-server -f\n'
  printf '  Configuration:     %s (0600)\n' "$CONF_PATH"
  printf '  Metrics (local):   curl -s http://127.0.0.1:9187/metrics (if enabled)\n'
  if [ "$WITH_IOS" -eq 1 ]; then
    printf '  iPhone (WireGuard): %s:%s - scan the QR code above\n' "$PUBLIC_IP" "$WG_PORT"
  fi
  printf '\n'
  printf 'Firewall reminder: the port UDP/%s must also be allowed by the cloud\n' "$PORT"
  printf 'provider (Oracle VCN security list, AWS security group, GCP firewall rule).\n'
  printf 'A host firewall alone does not make the server reachable.\n'

  if is_private_ip "$PUBLIC_IP"; then
    printf '\n'
    warn "the address above is private: the invite code will not work from outside."
    warn "Re-run with W1NWIRE_PUBLIC_HOST=<public ip> once the machine has a public address."
  fi

  printf '\nDocumentation: deploy/README.md, docs/SERVER.md, docs/ORACLE.md, docs/IOS.md\n'
}

# --- Argument parsing --------------------------------------------------------
parse_args() {
  while [ "$#" -gt 0 ]; do
    case "$1" in
      --port)
        [ "$#" -ge 2 ] || die "--port requires a value"
        PORT="$2"
        PORT_SET=1
        shift 2
        ;;
      --port=*)
        PORT="${1#*=}"
        PORT_SET=1
        shift
        ;;
      --repo)
        [ "$#" -ge 2 ] || die "--repo requires a value"
        REPO="$2"
        shift 2
        ;;
      --repo=*)
        REPO="${1#*=}"
        shift
        ;;
      --version)
        [ "$#" -ge 2 ] || die "--version requires a value"
        VERSION="$2"
        shift 2
        ;;
      --version=*)
        VERSION="${1#*=}"
        shift
        ;;
      --ios)
        WITH_IOS=1
        shift
        ;;
      --no-ios)
        WITH_IOS=0
        shift
        ;;
      --add-phone)
        ADD_PHONE=1
        WITH_IOS=1
        shift
        ;;
      --no-firewall)
        FIREWALL=0
        shift
        ;;
      --no-keepalive)
        KEEPALIVE=0
        shift
        ;;
      --from-source)
        FROM_SOURCE=1
        shift
        ;;
      --uninstall)
        DO_UNINSTALL=1
        shift
        ;;
      -h | --help)
        usage
        exit 0
        ;;
      --)
        shift
        break
        ;;
      *)
        error "unknown option: $1"
        printf '\n' >&2
        usage >&2
        exit 2
        ;;
    esac
  done

  case "$PORT" in
    '' | *[!0-9]*) die "--port must be a number, got '$PORT'" ;;
  esac
  if [ "$PORT" -lt 1 ] || [ "$PORT" -gt 65535 ]; then
    die "--port must be between 1 and 65535, got $PORT"
  fi

  case "$REPO" in
    */*/*) die "--repo must look like owner/repo, got '$REPO'" ;;
    */*) ;;
    *) die "--repo must look like owner/repo, got '$REPO'" ;;
  esac
  REPO="${REPO%/}"
  REPO="${REPO#/}"

  if [ "$VERSION" != latest ]; then
    case "$VERSION" in
      v*) ;;
      *) VERSION="v$VERSION" ;;
    esac
  fi
}

# --- Main --------------------------------------------------------------------
main() {
  parse_args "$@"

  if [ "$DO_UNINSTALL" -eq 1 ]; then
    require_root
    require_systemd
    WORKDIR=$(mktemp -d)
    do_uninstall
    exit 0
  fi

  if [ "$ADD_PHONE" -eq 1 ]; then
    require_root
    require_systemd
    WORKDIR=$(mktemp -d)
    detect_public_ip
    add_phone_peer
    exit 0
  fi

  require_root
  require_systemd
  detect_os
  detect_arch

  WORKDIR=$(mktemp -d)
  info "work directory: $WORKDIR"

  step "installing the server"
  ensure_base_packages
  install_binary
  detect_public_ip
  detect_wan_if

  ensure_directories
  ensure_keys
  ensure_config
  ensure_first_peer

  install_unit
  enable_forwarding
  install_nat_rules
  open_udp_port "$PORT"
  persist_rules

  start_service
  check_metrics

  install_keepalive

  if [ "$WITH_IOS" -eq 1 ]; then
    setup_ios
  fi

  # A changed listen port (or anything else the operator patched) is only
  # picked up by a restart; do it before the summary claims the server is up.
  if [ "$RESTART_NEEDED" -eq 1 ]; then
    info "the configuration changed, restarting the service"
    systemctl restart w1nwire-server.service
    if wait_for_active w1nwire-server.service; then
      info "w1nwire-server is active"
    else
      show_failure w1nwire-server.service
    fi
  fi

  print_summary
}

main "$@"
