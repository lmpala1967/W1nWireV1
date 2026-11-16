#!/usr/bin/env bash
# =============================================================================
# W1nWire — client configuration generator.
#
# Renders a ready-to-use client.json from the key material produced by
# `deploy/scripts/gen-keys.sh`.
#
# ---------------------------------------------------------------------------
# CONFIGURATION FORMAT (JSON)
# ---------------------------------------------------------------------------
# The schema is the one of internal/config.ClientConfig - the JSON decoder of
# the application rejects unknown fields, so the file must match it exactly:
#
# {
#   "server": {
#     "address": "203.0.113.10:51820", // host and UDP port of the server
#     "public_key": "<base64>"          // public key of the server
#   },
#   "private_key": "<base64>",          // private key of this client
#   "tun": {
#     "enabled": true,
#     "name": "w1n0",
#     "address": "10.66.0.2/32",         // address inside the tunnel
#     "gateway": "10.66.0.1",            // address of the server
#     "mtu": 1420,
#     "configure_routes": true,          // install the default route
#     "route_all": false                 // keep the traffic to the server local
#   },
#   "socks5": {
#     "enabled": true,
#     "listen": "127.0.0.1:1080"         // local proxy
#   },
#   "dns": "1.1.1.1",
#   "http": {
#     "metrics": {"enabled": true, "host": "127.0.0.1", "port": 9188}
#   }
# }
#
# Usage:
#   ./make-client-conf.sh \
#     --server-host 203.0.113.10 \
#     --server-port 51820 \
#     --client-private-key ./keys/client.key \
#     --server-public-key ./server.pub \
#     --client-public-key ./keys/client.pub \
#     --out ./client.json
# =============================================================================

set -euo pipefail

SERVER_HOST=""
SERVER_PORT="51820"
CLIENT_KEY=""
SERVER_KEY=""
CLIENT_PUB_KEY=""
OUT_FILE="./client.json"
ADDRESS=""
GATEWAY=""
TUNNEL_CIDR="10.66.0.0/24"
TUN_NAME="w1n0"
MTU="1420"
SOCKS5="127.0.0.1:1080"
DNS="1.1.1.1"
METRICS_PORT="9188"
ROUTE_ALL="false"
NAME="client"

usage() {
  cat <<'EOF'
Usage: make-client-conf.sh --server-host HOST [options]

Required:
  --server-host HOST          Public address of the W1nWire server
  --client-private-key FILE   File with the client private key (base64)
  --server-public-key FILE    File with the server public key (base64)

Optional:
  --server-port PORT          UDP port of the server (default: 51820)
  --out FILE                  Output file (default: ./client.json)
  --address CIDR              Address inside the tunnel (default: 10.66.0.2/32)
  --gateway IP                Address of the server inside the tunnel
  --tunnel-cidr CIDR          Network behind the server (default: 10.66.0.0/24)
  --tun-name NAME             Name of the local tunnel device (default: w1n0)
  --mtu N                     Tunnel MTU (default: 1420)
  --socks5 ADDR               Local SOCKS5 proxy, empty to disable it
                              (default: 127.0.0.1:1080)
  --dns IP                    Resolver used inside the tunnel (default: 1.1.1.1)
  --metrics-port N            Port of the Prometheus endpoint (default: 9188)
  --route-all                 Route *all* traffic through the tunnel
  --client-public-key FILE    File with the client public key (base64).
                              Optional - when given, an additional file
                              "<out>.entry.json" is written. It contains the
                              peer entry that has to be added to the peers
                              array of the server configuration.
  --name NAME                 Name of the client (default: client)
  -h, --help                  Show this help
EOF
}

die() {
  printf '[make-client-conf] ERROR: %s\n' "$*" >&2
  exit 2
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --server-host)
      [ "$#" -ge 2 ] || die "--server-host requires a value"
      SERVER_HOST="$2"
      shift 2
      ;;
    --server-port)
      [ "$#" -ge 2 ] || die "--server-port requires a value"
      SERVER_PORT="$2"
      shift 2
      ;;
    --client-private-key)
      [ "$#" -ge 2 ] || die "--client-private-key requires a value"
      CLIENT_KEY="$2"
      shift 2
      ;;
    --server-public-key)
      [ "$#" -ge 2 ] || die "--server-public-key requires a value"
      SERVER_KEY="$2"
      shift 2
      ;;
    --client-public-key)
      [ "$#" -ge 2 ] || die "--client-public-key requires a value"
      CLIENT_PUB_KEY="$2"
      shift 2
      ;;
    --out)
      [ "$#" -ge 2 ] || die "--out requires a value"
      OUT_FILE="$2"
      shift 2
      ;;
    --address)
      [ "$#" -ge 2 ] || die "--address requires a value"
      ADDRESS="$2"
      shift 2
      ;;
    --gateway)
      [ "$#" -ge 2 ] || die "--gateway requires a value"
      GATEWAY="$2"
      shift 2
      ;;
    --tunnel-cidr)
      [ "$#" -ge 2 ] || die "--tunnel-cidr requires a value"
      TUNNEL_CIDR="$2"
      shift 2
      ;;
    --tun-name)
      [ "$#" -ge 2 ] || die "--tun-name requires a value"
      TUN_NAME="$2"
      shift 2
      ;;
    --mtu)
      [ "$#" -ge 2 ] || die "--mtu requires a value"
      MTU="$2"
      shift 2
      ;;
    --socks5)
      [ "$#" -ge 2 ] || die "--socks5 requires a value"
      SOCKS5="$2"
      shift 2
      ;;
    --dns)
      [ "$#" -ge 2 ] || die "--dns requires a value"
      DNS="$2"
      shift 2
      ;;
    --metrics-port)
      [ "$#" -ge 2 ] || die "--metrics-port requires a value"
      METRICS_PORT="$2"
      shift 2
      ;;
    --route-all)
      ROUTE_ALL="true"
      shift
      ;;
    --name)
      [ "$#" -ge 2 ] || die "--name requires a value"
      NAME="$2"
      shift 2
      ;;
    -h | --help)
      usage
      exit 0
      ;;
    *)
      printf '[make-client-conf] unknown argument: %s\n' "$1" >&2
      usage
      exit 1
      ;;
  esac
done

[ -n "$SERVER_HOST" ] || {
  usage
  die "--server-host is required"
}
[ -n "$CLIENT_KEY" ] || die "--client-private-key is required"
[ -n "$SERVER_KEY" ] || die "--server-public-key is required"

[ -f "$CLIENT_KEY" ] || die "client private key '$CLIENT_KEY' not found"
[ -f "$SERVER_KEY" ] || die "server public key '$SERVER_KEY' not found"

# --- Read and validate the key material --------------------------------------
# The key files contain a single base64 line (with or without padding).
read_key() {
  local file="$1" value padded decoded
  value=$(tr -d ' \t\r\n' < "$file")
  [ -n "$value" ] || die "'$file' is empty"

  # Re-add the padding that the unpadded base64 of the Go implementation
  # does not have, so that the file can be decoded with any tool.
  padded="$value"
  case $((${#padded} % 4)) in
    2) padded="${padded}==" ;;
    3) padded="${padded}=" ;;
  esac

  decoded=$(printf '%s' "$padded" | base64 -d 2> /dev/null | wc -c | tr -d ' ')
  if [ "$decoded" != "32" ]; then
    # BSD base64 (macOS) spells the flag differently.
    decoded=$(printf '%s' "$padded" | base64 -D 2> /dev/null | wc -c | tr -d ' ')
  fi
  if [ "$decoded" != "32" ]; then
    die "'$file' does not contain a 32 byte base64 encoded X25519 key (decoded $decoded bytes)"
  fi
  echo "$value"
}

PRIVATE_KEY=$(read_key "$CLIENT_KEY")
SERVER_PUBLIC_KEY=$(read_key "$SERVER_KEY")

# --- Derive the defaults ------------------------------------------------------
# The server takes the first host of the tunnel network, the first client the
# second one - the same convention the `w1nwire-client gen-config` command
# uses.
if [ -z "$ADDRESS" ]; then
  ADDRESS="${TUNNEL_CIDR%/*}.2/32"
fi
if [ -z "$GATEWAY" ]; then
  GATEWAY="${TUNNEL_CIDR%/*}.1"
fi
if [ -n "$SOCKS5" ]; then
  SOCKS5_ENABLED="true"
else
  SOCKS5_ENABLED="false"
  SOCKS5="127.0.0.1:1080"
fi

# --- Render the configuration -------------------------------------------------
cat > "$OUT_FILE" <<EOF
{
  "server": {
    "address": "$SERVER_HOST:$SERVER_PORT",
    "public_key": "$SERVER_PUBLIC_KEY"
  },
  "private_key": "$PRIVATE_KEY",
  "tun": {
    "enabled": true,
    "name": "$TUN_NAME",
    "address": "$ADDRESS",
    "gateway": "$GATEWAY",
    "mtu": $MTU,
    "configure_routes": true,
    "route_all": $ROUTE_ALL
  },
  "socks5": {
    "enabled": $SOCKS5_ENABLED,
    "listen": "$SOCKS5"
  },
  "dns": "$DNS",
  "http": {
    "metrics": {
      "enabled": true,
      "host": "127.0.0.1",
      "port": $METRICS_PORT
    }
  }
}
EOF

chmod 0600 "$OUT_FILE"

# --- Server registration entry -------------------------------------------------
# When the client public key is known, the matching peer entry for the server
# side (the "peers" array of /etc/w1nwire/server.json) is generated as well.
if [ -n "$CLIENT_PUB_KEY" ]; then
  [ -f "$CLIENT_PUB_KEY" ] || die "client public key '$CLIENT_PUB_KEY' not found"
  CLIENT_PUBLIC_KEY=$(read_key "$CLIENT_PUB_KEY")
  ENTRY_FILE="${OUT_FILE%.json}.entry.json"
  cat > "$ENTRY_FILE" <<EOF
{
  "id": "$NAME",
  "public_key": "$CLIENT_PUBLIC_KEY",
  "tunnel_address": "${ADDRESS%/*}",
  "allowed_ips": ["$TUNNEL_CIDR"]
}
EOF
  chmod 0600 "$ENTRY_FILE"
  printf '[make-client-conf] server registration entry written to %s\n' "$ENTRY_FILE"
fi

printf '[make-client-conf] client configuration written to %s\n' "$OUT_FILE"
printf '[make-client-conf] endpoint: %s:%s\n' "$SERVER_HOST" "$SERVER_PORT"
printf '[make-client-conf] tunnel address: %s, gateway: %s\n' "$ADDRESS" "$GATEWAY"
printf '\nNext steps:\n'
printf '  1. add the peer entry to the "peers" array of the server configuration\n'
printf '     (/etc/w1nwire/server.json) and restart the server,\n'
printf '  2. copy this file to the client machine as /etc/w1nwire/client.json,\n'
printf '  3. install the unit:  sudo cp deploy/systemd/w1nwire-client.service /etc/systemd/system/\n'
printf '  4. start it:          sudo systemctl daemon-reload && sudo systemctl enable --now w1nwire-client\n'
