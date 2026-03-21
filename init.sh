#!/usr/bin/env bash

set -euo pipefail

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SERVICE_NAME="axisd.service"
SERVICE_FILE_SOURCE="${PROJECT_ROOT}/deployments/systemd/axisd.service"
SERVICE_FILE_TARGET="/etc/systemd/system/${SERVICE_NAME}"
AXISD_OUTPUT="${PROJECT_ROOT}/axisd"
AXIS_OUTPUT="${PROJECT_ROOT}/axis"
AXISD_TARGET="/usr/local/bin/axisd"
AXIS_TARGET="/usr/local/bin/axis"
ENV_FILE="${PROJECT_ROOT}/.env"
ENV_EXAMPLE_FILE="${PROJECT_ROOT}/.env.example"
REGION_MAPPING_FILE="${PROJECT_ROOT}/../NetStone/NetStone/conf/server_region_mapping.yaml"

if [[ ! -f "${PROJECT_ROOT}/README.md" ]]; then
  echo -e "${RED}Error:${NC} script must live in the Axis project root."
  exit 1
fi

if [[ ! -f "${SERVICE_FILE_SOURCE}" ]]; then
  echo -e "${RED}Error:${NC} missing systemd unit: ${SERVICE_FILE_SOURCE}"
  exit 1
fi

if [[ ! -f "${ENV_FILE}" ]]; then
  if [[ -f "${ENV_EXAMPLE_FILE}" ]]; then
    echo -e "${YELLOW}.env not found, copying from .env.example...${NC}"
    cp "${ENV_EXAMPLE_FILE}" "${ENV_FILE}"
  else
    echo -e "${RED}Error:${NC} missing ${ENV_FILE} and ${ENV_EXAMPLE_FILE}."
    exit 1
  fi
fi

if [[ "${EUID}" -eq 0 ]]; then
  SUDO=""
else
  if ! command -v sudo >/dev/null 2>&1; then
    echo -e "${RED}Error:${NC} sudo is required when not running as root."
    exit 1
  fi
  SUDO="sudo"
fi

run_root() {
  if [[ -n "${SUDO}" ]]; then
    ${SUDO} "$@"
  else
    "$@"
  fi
}

resolve_node_hostname() {
  if [[ -n "${HOST_HOSTNAME:-}" ]]; then
    printf '%s\n' "${HOST_HOSTNAME}"
    return 0
  fi
  if [[ -n "${HOSTNAME:-}" ]]; then
    printf '%s\n' "${HOSTNAME}"
    return 0
  fi
  if command -v hostname >/dev/null 2>&1; then
    hostname
    return 0
  fi
  return 1
}

detect_wt0_ipv4() {
  if ! command -v ip >/dev/null 2>&1; then
    return 1
  fi
  ip -o -4 addr show dev wt0 2>/dev/null | awk '{print $4}' | cut -d/ -f1 | head -n1
}

extract_hostname_prefix() {
  local hostname_value="$1"
  printf '%s\n' "${hostname_value}" | awk -F- '{print toupper($1)}'
}

upsert_env_value() {
  local key="$1"
  local value="$2"
  python3 - "${ENV_FILE}" "${key}" "${value}" <<'PY'
import pathlib, sys
env_path = pathlib.Path(sys.argv[1])
target_key = sys.argv[2]
target_value = sys.argv[3]
lines = env_path.read_text().splitlines()
updated = False
out = []
for line in lines:
    stripped = line.strip()
    if stripped and not stripped.startswith("#") and "=" in line:
        key, _ = line.split("=", 1)
        if key.strip() == target_key:
            out.append(f"{target_key}={target_value}")
            updated = True
            continue
    out.append(line)
if not updated:
    if out and out[-1] != "":
        out.append("")
    out.append(f"{target_key}={target_value}")
env_path.write_text("\n".join(out) + "\n")
PY
}

resolve_region_by_prefix() {
  local prefix="$1"
  [[ -f "${REGION_MAPPING_FILE}" ]] || return 1

  awk -v prefix="${prefix}" '
    $1 == "prefix_map:" { in_map = 1; next }
    in_map && $0 ~ /^  [A-Z0-9]+:$/ {
      if (entry == prefix && region != "") {
        found = 1
        print region
        exit
      }
      entry = $1
      sub(/:$/, "", entry)
      region = ""
      next
    }
    in_map && entry == prefix && $1 == "axis_region:" {
      region = $2
      next
    }
    END {
      if (!found && entry == prefix && region != "") {
        print region
      }
    }
  ' "${REGION_MAPPING_FILE}"
}

resolve_region_by_wt0_ip() {
  local wt0_ip="$1"
  python3 - "${ENV_FILE}" "${wt0_ip}" <<'PY'
import pathlib, sys

env_path = pathlib.Path(sys.argv[1])
wt0_ip = sys.argv[2].strip()
if not wt0_ip or not env_path.exists():
    raise SystemExit(0)

env = {}
for line in env_path.read_text().splitlines():
    line = line.strip()
    if not line or line.startswith("#") or "=" not in line:
        continue
    key, value = line.split("=", 1)
    env[key.strip()] = value.strip().strip('"').strip("'")

mapping = [
    ("north_america", "AXIS_WT0_REGION_NORTH_AMERICA_PREFIXES"),
    ("asia", "AXIS_WT0_REGION_ASIA_PREFIXES"),
    ("australia", "AXIS_WT0_REGION_AUSTRALIA_PREFIXES"),
    ("europe", "AXIS_WT0_REGION_EUROPE_PREFIXES"),
    ("south_america", "AXIS_WT0_REGION_SOUTH_AMERICA_PREFIXES"),
]

for region, key in mapping:
    raw = env.get(key, "")
    prefixes = [item.strip() for item in raw.split(",") if item.strip()]
    for prefix in prefixes:
        if wt0_ip.startswith(prefix):
            print(region)
            raise SystemExit(0)
PY
}

sync_local_region_from_mapping() {
  local hostname_value prefix region wt0_ip

  wt0_ip="$(detect_wt0_ipv4 || true)"
  if [[ -n "${wt0_ip}" ]]; then
    region="$(resolve_region_by_wt0_ip "${wt0_ip}" || true)"
    if [[ -n "${region}" && "${region}" != "null" ]]; then
      echo -e "${YELLOW}Detected wt0 IPv4:${NC} ${wt0_ip}"
      echo -e "${YELLOW}Updating AXIS_LOCAL_REGION in .env to:${NC} ${region}"
      upsert_env_value "AXIS_LOCAL_REGION" "${region}"
      return 0
    fi
    echo -e "${BLUE}wt0 IPv4 detected but no AXIS_WT0_REGION_* prefix matched; falling back to hostname mapping.${NC}"
  else
    echo -e "${BLUE}wt0 IPv4 not found; falling back to hostname mapping.${NC}"
  fi

  if [[ ! -f "${REGION_MAPPING_FILE}" ]]; then
    echo -e "${BLUE}Region mapping file not found; keeping existing AXIS_LOCAL_REGION in .env.${NC}"
    return 0
  fi

  hostname_value="$(resolve_node_hostname || true)"
  if [[ -z "${hostname_value}" ]]; then
    echo -e "${YELLOW}Hostname not resolved; keeping existing AXIS_LOCAL_REGION in .env.${NC}"
    return 0
  fi

  prefix="$(extract_hostname_prefix "${hostname_value}")"
  if [[ -z "${prefix}" ]]; then
    echo -e "${YELLOW}Hostname prefix is empty for ${hostname_value}; keeping existing AXIS_LOCAL_REGION in .env.${NC}"
    return 0
  fi

  region="$(resolve_region_by_prefix "${prefix}" || true)"
  if [[ -z "${region}" || "${region}" == "null" ]]; then
    echo -e "${YELLOW}No region mapping found for hostname prefix ${prefix}; keeping existing AXIS_LOCAL_REGION in .env.${NC}"
    return 0
  fi

  echo -e "${YELLOW}Resolved hostname prefix:${NC} ${prefix} (hostname: ${hostname_value})"
  echo -e "${YELLOW}Updating AXIS_LOCAL_REGION in .env to:${NC} ${region}"
  upsert_env_value "AXIS_LOCAL_REGION" "${region}"
}

echo -e "${CYAN}========================================${NC}"
echo -e "${CYAN}        Axis Update Installer           ${NC}"
echo -e "${CYAN}========================================${NC}"

echo -e "${BLUE}Project root:${NC} ${PROJECT_ROOT}"

sync_local_region_from_mapping

if ! command -v go >/dev/null 2>&1; then
  echo -e "${RED}Error:${NC} Go toolchain not found in PATH."
  exit 1
fi

if ! command -v systemctl >/dev/null 2>&1; then
  echo -e "${RED}Error:${NC} systemctl not found."
  exit 1
fi

echo -e "${YELLOW}Building latest axisd and axis binaries...${NC}"
(
  cd "${PROJECT_ROOT}"
  go build -o "${AXISD_OUTPUT}" ./cmd/axisd
  go build -o "${AXIS_OUTPUT}" ./cmd/axis
)

if run_root systemctl list-unit-files "${SERVICE_NAME}" >/dev/null 2>&1; then
  if run_root systemctl is-active --quiet "${SERVICE_NAME}"; then
    echo -e "${YELLOW}Stopping ${SERVICE_NAME} before replacing binaries...${NC}"
    run_root systemctl stop "${SERVICE_NAME}"
  else
    echo -e "${BLUE}${SERVICE_NAME} is not active.${NC}"
  fi
else
  echo -e "${BLUE}${SERVICE_NAME} is not installed yet.${NC}"
fi

echo -e "${YELLOW}Installing axisd to ${AXISD_TARGET}...${NC}"
run_root install -m 0755 "${AXISD_OUTPUT}" "${AXISD_TARGET}"

echo -e "${YELLOW}Installing axis CLI to ${AXIS_TARGET}...${NC}"
run_root install -m 0755 "${AXIS_OUTPUT}" "${AXIS_TARGET}"

echo -e "${YELLOW}Installing systemd unit to ${SERVICE_FILE_TARGET}...${NC}"
run_root install -m 0644 "${SERVICE_FILE_SOURCE}" "${SERVICE_FILE_TARGET}"

echo -e "${YELLOW}Reloading systemd daemon...${NC}"
run_root systemctl daemon-reload

echo -e "${YELLOW}Enabling and starting ${SERVICE_NAME}...${NC}"
run_root systemctl enable --now "${SERVICE_NAME}"

echo -e "${YELLOW}Verifying ${SERVICE_NAME} status...${NC}"
if run_root systemctl is-active --quiet "${SERVICE_NAME}"; then
  echo -e "${GREEN}${SERVICE_NAME} is active.${NC}"
else
  echo -e "${RED}${SERVICE_NAME} failed to start.${NC}"
  run_root systemctl status "${SERVICE_NAME}" --no-pager || true
  exit 1
fi

echo -e "${GREEN}Axis update applied successfully.${NC}"
echo -e "${BLUE}Daemon binary:${NC} ${AXISD_TARGET}"
echo -e "${BLUE}CLI binary:${NC} ${AXIS_TARGET}"
echo -e "${BLUE}Unit:${NC} ${SERVICE_FILE_TARGET}"
echo -e "${BLUE}Time:${NC} $(date '+%Y-%m-%d %H:%M:%S')"
