#!/usr/bin/env bash
# =============================================================================
# W1nWire — X25519 key pair generator.
#
# Generates the key material that is used by the W1nWire protocol:
#
#   * the SERVER key pair lives on the server (/etc/w1nwire/server.key and
#     server.pub), the public key is copied to every client;
#   * the CLIENT key pair lives on the client machine and is never shared.
#
# ---------------------------------------------------------------------------
# KEY FORMAT
# ---------------------------------------------------------------------------
# W1nWire uses X25519 (RFC 7748) as the key agreement primitive. The files
# contain the *raw* 32 byte keys encoded with base64 (RFC 4648, unpadded -
# exactly what `internal/crypto` produces and what `w1nwire-server keygen`
# prints):
#
#   <name>.key : the 32 byte private key  -> base64, e.g. "hM3x...9Q"
#   <name>.pub : the 32 byte public key   -> base64, e.g. "T7Kp...0w"
#
# This script only exists so that a key pair can be created on a machine where
# the W1nWire binary is not (yet) built - in that case OpenSSL is used as a
# fallback.
#
# Usage:
#   ./gen-keys.sh [--out DIR] [--name NAME] [--force] [--quiet]
#   ./gen-keys.sh --out ./keys --name client
#
# Exit codes: 0 = success, 1 = usage error, 2 = generation error.
# =============================================================================

set -euo pipefail

OUT_DIR="./keys"
NAME="client"
FORCE=0
QUIET=0
BINARY=""

usage() {
  cat <<'EOF'
Usage: gen-keys.sh [options]

Options:
  --out DIR      Directory for the generated files (default: ./keys)
  --name NAME    Name of the key pair (default: client)
  --force        Overwrite existing keys
  --quiet        Do not print the key material
  --binary PATH  Path to the w1nwire binary (default: autodetect)
  -h, --help     Show this help
EOF
}

log() {
  if [ "$QUIET" -eq 0 ]; then
    printf '[gen-keys] %s\n' "$*"
  fi
}

die() {
  printf '[gen-keys] ERROR: %s\n' "$*" >&2
  exit 2
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --out)
      [ "$#" -ge 2 ] || die "--out requires a directory"
      OUT_DIR="$2"
      shift 2
      ;;
    --name)
      [ "$#" -ge 2 ] || die "--name requires a value"
      NAME="$2"
      shift 2
      ;;
    --binary)
      [ "$#" -ge 2 ] || die "--binary requires a path"
      BINARY="$2"
      shift 2
      ;;
    --force)
      FORCE=1
      shift
      ;;
    --quiet)
      QUIET=1
      shift
      ;;
    -h | --help)
      usage
      exit 0
      ;;
    *)
      printf '[gen-keys] unknown argument: %s\n' "$1" >&2
      usage
      exit 1
      ;;
  esac
done

# --- Locate the w1nwire binary ------------------------------------------------
# Both the server and the client implement the `keygen` sub command.
find_binary() {
  if [ -n "$BINARY" ]; then
    [ -x "$BINARY" ] || die "binary '$BINARY' is not executable"
    echo "$BINARY"
    return 0
  fi

  local candidate
  for candidate in \
    ./bin/w1nwire-server \
    ./bin/w1nwire-client \
    ./out/w1nwire-server \
    ./out/w1nwire-client \
    "$(command -v w1nwire-server 2> /dev/null || true)" \
    "$(command -v w1nwire-client 2> /dev/null || true)"; do
    if [ -n "$candidate" ] && [ -x "$candidate" ]; then
      echo "$candidate"
      return 0
    fi
  done
  return 1
}

# --- Key generation -----------------------------------------------------------
# Preferred path: the W1nWire binary itself.
#
#   w1nwire-server keygen -out <file>
#     -> writes {"private_key": "...", "public_key": "..."} as JSON
#   w1nwire-server keygen
#     -> prints "private_key = ..." / "public_key  = ..."
generate_with_binary() {
  local bin="$1" tmp_json raw priv pub

  tmp_json=$(mktemp)
  if "$bin" keygen -out "$tmp_json" > /dev/null 2>&1; then
    raw=$(tr -d '\n' < "$tmp_json")
    priv=$(printf '%s' "$raw" | sed -n 's/.*"private_key"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')
    pub=$(printf '%s' "$raw" | sed -n 's/.*"public_key"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')
    rm -f "$tmp_json"
    if [ -n "$priv" ] && [ -n "$pub" ]; then
      printf '%s\n' "$priv" > "$OUT_DIR/$NAME.key"
      printf '%s\n' "$pub" > "$OUT_DIR/$NAME.pub"
      return 0
    fi
  else
    rm -f "$tmp_json"
  fi

  # Fallback: the textual output of `keygen` without -out.
  raw=$("$bin" keygen 2> /dev/null || true)
  priv=$(printf '%s\n' "$raw" | sed -n 's/^private_key[[:space:]]*=[[:space:]]*//p' | head -n 1 | tr -d ' \r')
  pub=$(printf '%s\n' "$raw" | sed -n 's/^public_key[[:space:]]*=[[:space:]]*//p' | head -n 1 | tr -d ' \r')
  if [ -n "$priv" ] && [ -n "$pub" ]; then
    printf '%s\n' "$priv" > "$OUT_DIR/$NAME.key"
    printf '%s\n' "$pub" > "$OUT_DIR/$NAME.pub"
    return 0
  fi
  return 1
}

# Fallback: OpenSSL. The raw X25519 keys are the last 32 bytes of the
# DER encoded PKCS#8 / SubjectPublicKeyInfo structures. The padding of the
# base64 encoding is removed to match the format of the Go implementation.
generate_with_openssl() {
  local pem
  command -v openssl > /dev/null 2>&1 || return 1

  pem=$(mktemp)
  openssl genpkey -algorithm X25519 -out "$pem" > /dev/null 2>&1 || {
    rm -f "$pem"
    return 1
  }
  openssl pkey -in "$pem" -outform DER 2> /dev/null | tail -c 32 | base64 | tr -d '\n=' > "$OUT_DIR/$NAME.key"
  openssl pkey -in "$pem" -pubout -outform DER 2> /dev/null | tail -c 32 | base64 | tr -d '\n=' > "$OUT_DIR/$NAME.pub"
  rm -f "$pem"
  return 0
}

# --- Main ---------------------------------------------------------------------
mkdir -p "$OUT_DIR"

if [ -e "$OUT_DIR/$NAME.key" ] && [ "$FORCE" -ne 1 ]; then
  die "$OUT_DIR/$NAME.key already exists (use --force to overwrite)"
fi

log "generating the '$NAME' X25519 key pair in $OUT_DIR"

if BINARY_PATH=$(find_binary); then
  log "using the W1nWire binary: $BINARY_PATH"
  if ! generate_with_binary "$BINARY_PATH"; then
    log "the binary could not generate the key pair - falling back to OpenSSL"
    generate_with_openssl || die "key generation failed"
  fi
else
  log "no w1nwire binary found - using OpenSSL"
  generate_with_openssl || die "key generation failed (is OpenSSL installed?)"
fi

chmod 0600 "$OUT_DIR/$NAME.key"
chmod 0644 "$OUT_DIR/$NAME.pub"

# Machine readable summary.
cat > "$OUT_DIR/$NAME.json" <<EOF
{
  "name": "$NAME",
  "algorithm": "x25519",
  "private_key_file": "$NAME.key",
  "public_key_file": "$NAME.pub",
  "encoding": "base64(raw 32 byte key)"
}
EOF
chmod 0600 "$OUT_DIR/$NAME.json"

if [ "$QUIET" -eq 0 ]; then
  printf '\n'
  printf '  public key : %s\n' "$(cat "$OUT_DIR/$NAME.pub")"
  printf '  private key: %s\n' "$(cat "$OUT_DIR/$NAME.key")"
  printf '\n'
  printf '  The public key goes to the server, the private key stays here.\n'
  printf '  Format: base64 of the raw 32 byte X25519 key.\n'
fi
