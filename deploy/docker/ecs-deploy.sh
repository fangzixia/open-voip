#!/usr/bin/env bash
# Deploy open-switch + open-call on ECS via Docker Compose (host network).
# Postgres stays on the host; configs come from migrate-ecs-config.py.
set -euo pipefail

ROOT="${OPEN_VOIP_ROOT:-/opt/open-voip/src}"
ENV_FILE="${ENV_FILE:-/opt/open-voip/.env}"
COMPOSE_FILE="${COMPOSE_FILE:-${ROOT}/compose.yml}"

if [[ ! -f "${COMPOSE_FILE}" ]]; then
  echo "compose file not found: ${COMPOSE_FILE}" >&2
  exit 1
fi
if [[ ! -f "${ENV_FILE}" ]]; then
  echo "missing ${ENV_FILE}; run deploy/docker/migrate-ecs-config.py first" >&2
  exit 1
fi

# shellcheck disable=SC1090
set -a
source "${ENV_FILE}"
set +a

SWITCH_HEALTH_URL="${SWITCH_HEALTH_URL:-http://127.0.0.1:8082/health}"
CALL_HEALTH_URL="${CALL_HEALTH_URL:-http://127.0.0.1:8080/health/ready}"

echo "==> Checking PostgreSQL (host)..."
if command -v pg_isready >/dev/null 2>&1; then
  pg_isready -h 127.0.0.1 -p 5432 -U openvoip || {
    echo "PostgreSQL not ready on 127.0.0.1:5432" >&2
    exit 1
  }
else
  echo "pg_isready not installed; skipping DB probe"
fi

BACKUP_DIR="/opt/open-voip/backups/$(date +%Y%m%d-%H%M%S)"
mkdir -p "${BACKUP_DIR}"

if systemctl is-active --quiet open-switch 2>/dev/null; then
  echo "==> Stopping systemd open-switch..."
  systemctl stop open-switch
fi
if systemctl is-active --quiet open-call 2>/dev/null; then
  echo "==> Stopping systemd open-call..."
  systemctl stop open-call
fi

for svc in open-switch open-call; do
  if [[ -f "/opt/${svc}/config.yml" ]]; then
    cp -a "/opt/${svc}/config.yml" "${BACKUP_DIR}/${svc}-config.yml"
  fi
done

if docker compose version >/dev/null 2>&1; then
  COMPOSE=(docker compose)
else
  COMPOSE=(docker-compose)
fi

echo "==> Pulling images (skip if building locally)..."
if ! "${COMPOSE[@]}" --env-file "${ENV_FILE}" -f "${COMPOSE_FILE}" pull open-switch open-call; then
  echo "Pull failed; building images on host..."
  "${COMPOSE[@]}" --env-file "${ENV_FILE}" -f "${COMPOSE_FILE}" build open-switch open-call
fi

echo "==> Starting containers..."
"${COMPOSE[@]}" --env-file "${ENV_FILE}" -f "${COMPOSE_FILE}" up -d open-switch open-call

echo "==> Health checks..."
for _ in $(seq 1 30); do
  if curl --fail --silent --max-time 3 "${SWITCH_HEALTH_URL}" >/dev/null \
    && curl --fail --silent --max-time 3 "${CALL_HEALTH_URL}" >/dev/null; then
    echo "Switch and Call are healthy."
    break
  fi
  sleep 2
done

curl --fail --silent --max-time 5 "${SWITCH_HEALTH_URL}" >/dev/null || {
  echo "Switch health failed: ${SWITCH_HEALTH_URL}" >&2
  exit 1
}
curl --fail --silent --max-time 5 "${CALL_HEALTH_URL}" >/dev/null || {
  echo "Call health failed: ${CALL_HEALTH_URL}" >&2
  exit 1
}

if systemctl list-unit-files open-switch.service >/dev/null 2>&1; then
  systemctl disable open-switch open-call 2>/dev/null || true
  echo "Disabled systemd units open-switch/open-call (use docker compose for upgrades)."
fi

echo "Done. Upgrade with: IMAGE_TAG=<tag> in ${ENV_FILE}, then re-run this script."
