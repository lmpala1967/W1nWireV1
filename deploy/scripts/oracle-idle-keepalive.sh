#!/usr/bin/env bash
# =============================================================================
# W1nWire — Oracle Cloud idle keepalive.
#
# WHY THIS FILE EXISTS
# --------------------
# An Oracle Cloud "Always Free" compute instance can be reclaimed when it stays
# idle: Oracle's free tier documentation says that an Always Free compute
# instance may be stopped when, over a seven day window, its CPU, network and
# memory utilization stay below the thresholds. A VPN exit node that nobody
# uses sits at almost zero on all three metrics, so a free instance can be
# stopped - and its boot volume eventually reclaimed - even though nothing is
# wrong with it.
#
# This job is a cheap floor under those metrics. Every five minutes it
#
#   * burns CPU for a few seconds      (dd from /dev/zero to /dev/null),
#   * allocates a few MiB of RAM       (a file in /dev/shm, removed again),
#   * moves a little traffic           (one HTTPS request, a few hundred KiB).
#
# It is deliberately small: it must not cost money, must not compete with the
# VPN for CPU, and must not look like a fork bomb on a 1 OCPU instance. With
# the defaults (20 s of CPU every 5 minutes) it keeps roughly 6-7 % of one
# core busy, and the whole cycle finishes in about half a minute.
#
# HONEST LIMITATION
# -----------------
# The published rule talks about utilization over a week, so a short periodic
# spike cannot make an unused instance look busy: it only keeps the instance
# out of the "nothing happened at all" bucket. The real defense is to actually
# use the VPN. See docs/ORACLE.md for what the console shows and how to check
# whether the instance is still classified as free.
#
# WHAT INSTALLS IT
# ----------------
# deploy/install.sh copies this file to /usr/local/bin/w1nwire-keepalive.sh and
# enables w1nwire-keepalive.timer (every five minutes). It can also be run by
# hand:
#
#   ./oracle-idle-keepalive.sh            # one cycle now
#   ./oracle-idle-keepalive.sh --dry-run  # print the plan, do nothing
#   ./oracle-idle-keepalive.sh --help
#
# Tunables (environment variables, or a shell fragment in
# /etc/default/w1nwire-keepalive that sets them):
#
#   W1NWIRE_KEEPALIVE_CPU_SECONDS   default 20
#   W1NWIRE_KEEPALIVE_MEM_MIB       default 16
#   W1NWIRE_KEEPALIVE_NET_BYTES     default 262144 (256 KiB)
# =============================================================================
set -euo pipefail

CPU_SECONDS="${W1NWIRE_KEEPALIVE_CPU_SECONDS:-20}"
MEM_MIB="${W1NWIRE_KEEPALIVE_MEM_MIB:-16}"
NET_BYTES="${W1NWIRE_KEEPALIVE_NET_BYTES:-262144}"
DRY_RUN=0

# A few reliable, free endpoints. The first one honours the byte count, the
# others are only a fallback for when it is unreachable from the region.
KEEPALIVE_URLS=(
  "https://speed.cloudflare.com/__down?bytes=${NET_BYTES}"
  "https://cloudflare.com/cdn-cgi/trace"
  "https://connectivitycheck.gstatic.com/generate_204"
)

log() {
  printf '%s w1nwire-keepalive: %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*"
}

require_uint() {
  local name="$1" value="$2"
  case "$value" in
    '' | *[!0-9]*)
      log "WARNING: $name must be a positive integer, got '$value' - using the default"
      return 1
      ;;
  esac
  return 0
}

# --- CPU ---------------------------------------------------------------------
# dd copying zeroes from /dev/zero to /dev/null is the cheapest portable way to
# keep a core busy without allocating anything: no fork bomb, no temporary
# files, no measurable memory footprint. The loop is bounded by a wall clock
# deadline so the job can never run away, whatever dd does.
burn_cpu() {
  if ! require_uint W1NWIRE_KEEPALIVE_CPU_SECONDS "$CPU_SECONDS" || [ "$CPU_SECONDS" -eq 0 ]; then
    return 0
  fi
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

# --- Memory ------------------------------------------------------------------
# The utilization metric of the instance is about memory that is actually in
# use. /dev/shm is tmpfs, so a file written there lives in RAM and is visible
# to the metric for as long as it exists; it is removed in the same cycle.
touch_memory() {
  if ! require_uint W1NWIRE_KEEPALIVE_MEM_MIB "$MEM_MIB" || [ "$MEM_MIB" -eq 0 ]; then
    return 0
  fi
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
  rm -f "$file"
}

# --- Network -----------------------------------------------------------------
# Both directions matter: the HTTP request is a few hundred KiB down, and the
# TCP handshake plus the ACKs are up. That is far below 10 % of a gigabit link,
# which is exactly why the honest limitation above exists - the point is to
# have non-zero traffic, not to fake a busy server.
touch_network() {
  if ! require_uint W1NWIRE_KEEPALIVE_NET_BYTES "$NET_BYTES" || [ "$NET_BYTES" -eq 0 ]; then
    return 0
  fi
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

  # Last resort: a DNS lookup still produces a little traffic.
  if command -v getent > /dev/null 2>&1; then
    getent hosts one.one.one.one > /dev/null 2>&1 || true
  fi
  log "WARNING: no keepalive endpoint was reachable"
  return 0
}

usage() {
  cat <<'USAGE'
w1nwire-keepalive - keep an Oracle Cloud Always Free instance out of the idle bucket

Usage:
  w1nwire-keepalive.sh            run one keepalive cycle
  w1nwire-keepalive.sh --dry-run  print what a cycle would do, change nothing
  w1nwire-keepalive.sh --help     this text

Installed by deploy/install.sh as /usr/local/bin/w1nwire-keepalive.sh and run
by w1nwire-keepalive.timer every five minutes.

Tunables (environment, or /etc/default/w1nwire-keepalive):
  W1NWIRE_KEEPALIVE_CPU_SECONDS   seconds of CPU load per cycle   (default 20)
  W1NWIRE_KEEPALIVE_MEM_MIB       MiB allocated in /dev/shm       (default 16)
  W1NWIRE_KEEPALIVE_NET_BYTES     bytes downloaded per cycle      (default 262144)
USAGE
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
