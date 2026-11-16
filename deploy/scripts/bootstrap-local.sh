#!/usr/bin/env bash
# =============================================================================
# W1nWire — local development bootstrap.
#
# Builds and runs the W1nWire server on the developer machine, either
# natively (recommended: faster iteration, direct access to the source) or
# inside Docker (reproducible, close to the OCI deployment).
#
# ---------------------------------------------------------------------------
# NATIVE MODE
# ---------------------------------------------------------------------------
#   1. `go build` the server (and optionally the client) into ./.local/bin
#   2. generate the server key pair into ./.local
#   3. write ./.local/server.json (same schema as on the server)
#   4. run the server in the foreground
#
# Creating a TUN device requires CAP_NET_ADMIN - on a normal workstation the
# script has to be executed with sudo (or the capability has to be granted:
# `sudo setcap cap_net_admin+ep ./local/bin/w1nwire-server`).
#
# ---------------------------------------------------------------------------
# DOCKER MODE
# ---------------------------------------------------------------------------
# The image is built from the repository (multi stage: golang builder +
# alpine/distroless runtime) and executed with `--cap-add NET_ADMIN` and
# `--device /dev/net/tun`.
#
# Usage:
#   ./bootstrap-local.sh                     # native build + run
#   ./bootstrap-local.sh --docker            # docker build + run
#   ./bootstrap-local.sh --build-only        # only build
#   ./bootstrap-local.sh --client            # build the client as well
#   ./bootstrap-local.sh --port 51821
#   ./bootstrap-local.sh --install-service   # install the systemd units (root)
# =============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
LOCAL_DIR="$REPO_ROOT/.local"
BIN_DIR="$LOCAL_DIR/bin"
CONF_DIR="$LOCAL_DIR/conf"
DOCKER_DIR="$LOCAL_DIR/docker"
IMAGE_NAME="w1nwire:local"
CONTAINER_NAME="w1nwire-server"

MODE="native"
PORT="51820"
TUNNEL_CIDR="10.66.0.0/24"
TUN_IFACE="w1n0"
BUILD_ONLY=0
BUILD_CLIENT=0
INSTALL_SERVICE=0
usage() {
  sed -n '2,40p' "$0"
}

log() {
  printf '[bootstrap-local] %s\n' "$*"
}

die() {
  printf '[bootstrap-local] ERROR: %s\n' "$*" >&2
  exit 1
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --docker)
      MODE="docker"
      shift
      ;;
    --native)
      MODE="native"
      shift
      ;;
    --port)
      [ "$#" -ge 2 ] || die "--port requires a value"
      PORT="$2"
      shift 2
      ;;
    --tunnel-cidr)
      [ "$#" -ge 2 ] || die "--tunnel-cidr requires a value"
      TUNNEL_CIDR="$2"
      shift 2
      ;;
    --tun)
      [ "$#" -ge 2 ] || die "--tun requires a value"
      TUN_IFACE="$2"
      shift 2
      ;;
    --build-only)
      BUILD_ONLY=1
      shift
      ;;
    --client)
      BUILD_CLIENT=1
      shift
      ;;
    --install-service)
      INSTALL_SERVICE=1
      shift
      ;;
    -h | --help)
      usage
      exit 0
      ;;
    *)
      printf '[bootstrap-local] unknown argument: %s\n' "$1" >&2
      usage
      exit 1
      ;;
  esac
done

# --- Helpers -------------------------------------------------------------------
# Addresses inside the tunnel: the server takes the first host of the network,
# the client the second one (the same convention the W1nWire configuration
# uses).
TUN_SERVER_IP="${TUNNEL_CIDR%/*}.1"
TUN_CLIENT_IP="${TUNNEL_CIDR%/*}.2"
TUN_SERVER_CIDR="$TUN_SERVER_IP/${TUNNEL_CIDR#*/}"

find_cmd_dir() {
  local pattern="$1" candidate
  for candidate in "$REPO_ROOT"/cmd/*"$pattern"*; do
    if [ -d "$candidate" ]; then
      echo "${candidate#$REPO_ROOT/}"
      return 0
    fi
  done
  return 1
}

require_go() {
  command -v go > /dev/null 2>&1 || die "Go is not installed (https://go.dev/dl/)"
}

# --- Build ---------------------------------------------------------------------
build_native() {
  require_go
  mkdir -p "$BIN_DIR" "$CONF_DIR"

  local server_pkg client_pkg
  server_pkg=$(find_cmd_dir "server") || die "no cmd/*server* package found in $REPO_ROOT/cmd"
  client_pkg=$(find_cmd_dir "client") || client_pkg=""

  log "building $server_pkg"
  ( cd "$REPO_ROOT" && go build -trimpath -o "$BIN_DIR/w1nwire-server" "$server_pkg" )

  if [ "$BUILD_CLIENT" -eq 1 ]; then
    [ -n "$client_pkg" ] || die "no cmd/*client* package found"
    log "building $client_pkg"
    ( cd "$REPO_ROOT" && go build -trimpath -o "$BIN_DIR/w1nwire-client" "$client_pkg" )
  fi
}

# --- Configuration -------------------------------------------------------------
# The schema is the one of internal/config.ServerConfig - the JSON decoder of
# the application rejects unknown fields.
prepare_config() {
  mkdir -p "$CONF_DIR"

  if [ ! -f "$CONF_DIR/server.key" ] || [ ! -f "$CONF_DIR/server.pub" ]; then
    log "generating the local server key pair"
    "$SCRIPT_DIR/gen-keys.sh" --out "$CONF_DIR" --name server --quiet
  else
    log "reusing the existing local key pair"
  fi

  if [ ! -f "$CONF_DIR/client.key" ] || [ ! -f "$CONF_DIR/client.pub" ]; then
    log "generating the local client key pair"
    "$SCRIPT_DIR/gen-keys.sh" --out "$CONF_DIR" --name client --quiet
  fi

  local server_key server_pub client_pub
  server_key=$(tr -d '\n' < "$CONF_DIR/server.key")
  server_pub=$(tr -d '\n' < "$CONF_DIR/server.pub")
  client_pub=$(tr -d '\n' < "$CONF_DIR/client.pub")

  cat > "$CONF_DIR/server.json" <<EOF
{
  "listen": "0.0.0.0:$PORT",
  "server_private_key": "$server_key",
  "tun": {
    "enabled": true,
    "name": "$TUN_IFACE",
    "address": "$TUN_SERVER_CIDR",
    "mtu": 1420
  },
  "nat": {
    "enabled": false,
    "outbound_interface": ""
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
      "id": "local",
      "public_key": "$client_pub",
      "tunnel_address": "$TUN_CLIENT_IP",
      "allowed_ips": ["$TUNNEL_CIDR"]
    }
  ]
}
EOF
  chmod 0600 "$CONF_DIR/server.json"

  log "configuration written to $CONF_DIR/server.json"
}

# --- systemd -------------------------------------------------------------------
install_units() {
  [ "$(id -u)" -eq 0 ] || die "--install-service has to be run as root"
  install -m 0644 "$REPO_ROOT/deploy/systemd/w1nwire-server.service" /etc/systemd/system/w1nwire-server.service
  install -m 0644 "$REPO_ROOT/deploy/systemd/w1nwire-client.service" /etc/systemd/system/w1nwire-client.service
  systemctl daemon-reload
  log "systemd units installed (the server unit expects /opt/w1nwire - adjust ExecStart for local use)"
}

# --- Docker --------------------------------------------------------------------
build_docker() {
  command -v docker > /dev/null 2>&1 || die "docker is not installed"
  mkdir -p "$DOCKER_DIR"

  local server_pkg
  server_pkg=$(find_cmd_dir "server") || die "no cmd/*server* package found in $REPO_ROOT/cmd"

  cat > "$DOCKER_DIR/Dockerfile" <<EOF
# W1nWire - local development image.
FROM golang:1.24-alpine AS builder
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download || true
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/w1nwire-server $server_pkg

FROM alpine:3.20
# iproute2 and iptables are needed by the server (it configures the tunnel
# device and the masquerading rule itself).
RUN apk add --no-cache ca-certificates iptables iproute2 && adduser -D -H w1nwire
COPY --from=builder /out/w1nwire-server /opt/w1nwire/bin/w1nwire-server
COPY deploy/systemd /etc/systemd/system
ENTRYPOINT ["/opt/w1nwire/bin/w1nwire-server"]
CMD ["--config", "/etc/w1nwire/server.json"]
EOF

  log "building the docker image $IMAGE_NAME"
  docker build -f "$DOCKER_DIR/Dockerfile" -t "$IMAGE_NAME" "$REPO_ROOT"
}

run_docker() {
  command -v docker > /dev/null 2>&1 || die "docker is not installed"

  if [ ! -c /dev/net/tun ]; then
    log "WARNING: /dev/net/tun does not exist on this host - the tunnel cannot be created"
  fi

  docker rm -f "$CONTAINER_NAME" > /dev/null 2>&1 || true
  log "starting the container $CONTAINER_NAME"
  docker run --rm -it \
    --name "$CONTAINER_NAME" \
    --cap-add NET_ADMIN \
    --device /dev/net/tun \
    --sysctl net.ipv4.ip_forward=1 \
    -p "$PORT:$PORT/udp" \
    -v "$CONF_DIR:/etc/w1nwire" \
    "$IMAGE_NAME" --config /etc/w1nwire/server.json
}

# --- Native run ----------------------------------------------------------------
run_native() {
  prepare_config
  log "starting the server on 0.0.0.0:$PORT"
  exec "$BIN_DIR/w1nwire-server" --config "$CONF_DIR/server.json"
}

# --- Main ----------------------------------------------------------------------
mkdir -p "$LOCAL_DIR"

case "$MODE" in
  native)
    build_native
    prepare_config
    if [ "$INSTALL_SERVICE" -eq 1 ]; then
      install_units
    fi
    if [ "$BUILD_ONLY" -eq 1 ]; then
      log "build finished"
    else
      run_native
    fi
    ;;
  docker)
    prepare_config
    build_docker
    if [ "$BUILD_ONLY" -eq 1 ]; then
      log "build finished"
    else
      run_docker
    fi
    ;;
  *)
    die "unknown mode '$MODE'"
    ;;
esac
