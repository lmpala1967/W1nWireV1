#!/usr/bin/env bash
#
# check-deps.sh — verify that the external tools required by W1nWire are
# installed and reachable.
#
# Usage:
#   ./scripts/check-deps.sh          # report status
#   ./scripts/check-deps.sh --quiet  # only print missing tools
#
# Exit code:
#   0  -> Go (mandatory) is present
#   1  -> Go is missing (or an unexpected error occurred)
#
# Missing optional tools (curl / jq / terraform / docker) only produce a
# warning and do not change the exit code.

set -u

REQUIRED_TOOL="go"
OPTIONAL_TOOLS="curl jq terraform docker"

QUIET=0
for arg in "$@"; do
  case "$arg" in
    --quiet|-q) QUIET=1 ;;
    -h|--help)
      echo "Usage: $(basename "$0") [--quiet]"
      exit 0
      ;;
    *)
      echo "Unknown argument: $arg" >&2
      exit 1
      ;;
  esac
done

RED=$'\033[31m'
GREEN=$'\033[32m'
YELLOW=$'\033[33m'
CYAN=$'\033[36m'
RESET=$'\033[0m'

if [ ! -t 1 ]; then
  RED=""; GREEN=""; YELLOW=""; CYAN=""; RESET=""
fi

found=0
missing_required=0
missing_optional=0

# check_tool <name> <required|optional>
check_tool() {
  local name="$1"
  local kind="$2"
  local path=""

  if command -v "$name" >/dev/null 2>&1; then
    path="$(command -v "$name")"
    local version=""
    case "$name" in
      go)      version="$(go version 2>/dev/null | awk '{print $3}')" ;;
      curl)    version="$(curl --version 2>/dev/null | awk 'NR==1{print $2}')" ;;
      jq)      version="$(jq --version 2>/dev/null)" ;;
      terraform) version="$(terraform version -json 2>/dev/null | grep -m1 'terraform_version' | cut -d'"' -f4)" ;;
      docker)  version="$(docker --version 2>/dev/null | awk '{print $3}' | tr -d ',')" ;;
    esac
    found=$((found + 1))
    [ "$QUIET" -eq 1 ] || printf '%s[ OK ]%s %-10s %s %s\n' "$GREEN" "$RESET" "$name" "$version" "$path"
  else
    if [ "$kind" = "required" ]; then
      missing_required=$((missing_required + 1))
      printf '%s[MISSING]%s %-10s %srequired%s\n' "$RED" "$RESET" "$name" "$CYAN" "$RESET"
    else
      missing_optional=$((missing_optional + 1))
      printf '%s[MISSING]%s %-10s %soptional%s\n' "$YELLOW" "$RESET" "$name" "$CYAN" "$RESET"
    fi
  fi
}

echo "W1nWire — dependency check"
echo "----------------------------------------"

# Mandatory: the Go toolchain.
check_tool "$REQUIRED_TOOL" required

# Optional, but needed for the full workflow.
for tool in $OPTIONAL_TOOLS; do
  check_tool "$tool" optional
done

echo "----------------------------------------"

if [ "$missing_required" -gt 0 ]; then
  printf '%sERROR:%s %s is required to build W1nWire. Install it from https://go.dev/dl/\n' \
    "$RED" "$RESET" "$REQUIRED_TOOL"
  exit 1
fi

if [ "$missing_optional" -gt 0 ]; then
  printf '%sWARNING:%s %s optional tool(s) missing.\n' "$YELLOW" "$RESET" "$missing_optional"
  echo "  - curl    : used by the smoke tests and the health-check scripts"
  echo "  - jq      : used to pretty-print / parse JSON metrics and API responses"
  echo "  - terraform: used by the infrastructure code under deploy/"
  echo "  - docker  : used by 'make docker-build' and the monitoring stack"
  # Only the Go toolchain is fatal; the optional tools only produce a warning.
  exit 0
fi

printf '%sAll %s dependencies are present.%s\n' "$GREEN" "$found" "$RESET"
exit 0
