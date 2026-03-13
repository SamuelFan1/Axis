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

if [[ ! -f "${PROJECT_ROOT}/README.md" ]]; then
  echo -e "${RED}Error:${NC} script must live in the Axis project root."
  exit 1
fi

if [[ ! -f "${SERVICE_FILE_SOURCE}" ]]; then
  echo -e "${RED}Error:${NC} missing systemd unit: ${SERVICE_FILE_SOURCE}"
  exit 1
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

echo -e "${CYAN}========================================${NC}"
echo -e "${CYAN}        Axis Update Installer           ${NC}"
echo -e "${CYAN}========================================${NC}"

echo -e "${BLUE}Project root:${NC} ${PROJECT_ROOT}"

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
