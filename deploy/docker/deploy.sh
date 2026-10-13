#!/usr/bin/env bash
# Open VoIP Docker deploy (Switch + Call, host network, no Compose).
# Usage:
#   deploy.sh run switch|call     Recreate container (reads ENV_FILE / .env)
#   deploy.sh ecs switch|call|all ECS migrate: stop systemd, backup, pull, run, healthcheck
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

usage() {
  cat <<EOF
Usage: $(basename "$0") <run|ecs> <switch|call|all>

  run switch|call   Start or recreate one container (PULL=1 by default)
  ecs switch|call   Full ECS-style deploy for one service
  ecs all           Deploy Switch then Call (default if no args)

Environment:
  ENV_FILE          Default /opt/open-voip/.env, else \${REPO_ROOT}/.env
  MIN_DOCKER_VERSION  Minimum Engine version (default 20.10.0)

EOF
}

require_docker() {
  local min="${MIN_DOCKER_VERSION:-20.10.0}"

  if ! command -v docker >/dev/null 2>&1; then
    echo "未检测到 docker 命令。" >&2
    exit 1
  fi

  local server_ver=""
  if ! server_ver="$(docker version --format '{{.Server.Version}}' 2>/dev/null)" || [[ -z "${server_ver}" ]]; then
    if ! server_ver="$(docker info --format '{{.ServerVersion}}' 2>/dev/null)" || [[ -z "${server_ver}" ]]; then
      echo "无法连接 Docker 守护进程。" >&2
      exit 1
    fi
  fi

  local ver="${server_ver%%-*}"
  if [[ "$(printf '%s\n' "${min}" "${ver}" | sort -V | head -1)" != "${min}" ]]; then
    echo "Docker 版本过低：当前 ${server_ver}，需要 >= ${min}。" >&2
    exit 1
  fi
}

load_env() {
  ENV_FILE="${ENV_FILE:-/opt/open-voip/.env}"
  if [[ ! -f "${ENV_FILE}" && -f "${REPO_ROOT}/.env" ]]; then
    ENV_FILE="${REPO_ROOT}/.env"
  fi
  if [[ -f "${ENV_FILE}" ]]; then
    # shellcheck disable=SC1090
    set -a
    source "${ENV_FILE}"
    set +a
  fi

  GHCR_REGISTRY="${GHCR_REGISTRY:-ghcr.io/fangzixia}"
  IMAGE_TAG="${IMAGE_TAG:-main}"
  PULL="${PULL:-1}"
  SWITCH_STATE_DIR="${SWITCH_STATE_DIR:-${REPO_ROOT}/runtime/open-switch}"
  CALL_STATE_DIR="${CALL_STATE_DIR:-${REPO_ROOT}/runtime/open-call}"
  SWITCH_UID="${SWITCH_UID:-10001}"
  SWITCH_GID="${SWITCH_GID:-10001}"
  WEB_LISTEN="${WEB_LISTEN:-0.0.0.0:8088}"
  CALL_HTTP_UPSTREAM="${CALL_HTTP_UPSTREAM:-127.0.0.1:8080}"
  SWITCH_HEALTH_URL="${SWITCH_HEALTH_URL:-http://127.0.0.1:8082/health}"
  CALL_HEALTH_URL="${CALL_HEALTH_URL:-http://127.0.0.1:8080/health/ready}"
  WEB_HEALTH_URL="${WEB_HEALTH_URL:-http://127.0.0.1:8088/container-health}"
}

require_env_file() {
  local try="${ENV_FILE:-/opt/open-voip/.env}"
  if [[ ! -f "${try}" && -f "${REPO_ROOT}/.env" ]]; then
    try="${REPO_ROOT}/.env"
  fi
  if [[ ! -f "${try}" ]]; then
    echo "missing ${try}; copy from .env.example and set SWITCH_STATE_DIR / CALL_STATE_DIR" >&2
    exit 1
  fi
  ENV_FILE="${try}"
}

run_switch() {
  local container="${SWITCH_CONTAINER_NAME:-open-switch}"
  local image="${GHCR_REGISTRY}/open-voip-open-switch:${IMAGE_TAG}"

  if [[ "${PULL}" == "1" ]]; then
    docker pull "${image}" || true
  fi

  docker rm -f "${container}" 2>/dev/null || true

  local -a args=(
    -d --name "${container}"
    --restart unless-stopped
    --init
    --network host
    --security-opt no-new-privileges
    --user "${SWITCH_UID}:${SWITCH_GID}"
    --ulimit "nofile=65536:65536"
    -e TZ=Asia/Hong_Kong
    -v "${SWITCH_STATE_DIR}:/opt/open-switch"
    -v "${SWITCH_STATE_DIR}/config.yml:/opt/open-switch/config.yml:ro"
    --log-opt max-size=20m
    --log-opt max-file=5
  )
  if [[ -n "${SWITCH_BINARY_MOUNT:-}" && -f "${SWITCH_BINARY_MOUNT}" ]]; then
    args+=(-v "${SWITCH_BINARY_MOUNT}:/usr/local/bin/open-switch:ro")
  fi

  docker run "${args[@]}" "${image}"
  echo "Started ${container} from ${image}"
}

run_call() {
  local container="${CALL_CONTAINER_NAME:-open-call}"
  local image="${GHCR_REGISTRY}/open-voip-open-call:${IMAGE_TAG}"

  if [[ "${PULL}" == "1" ]]; then
    docker pull "${image}" || true
  fi

  docker rm -f "${container}" 2>/dev/null || true

  local -a args=(
    -d --name "${container}"
    --restart unless-stopped
    --init
    --network host
    --security-opt no-new-privileges
    --ulimit "nofile=65536:65536"
    -e TZ=Asia/Hong_Kong
    -e "WEB_LISTEN=${WEB_LISTEN}"
    -e "CALL_HTTP_UPSTREAM=${CALL_HTTP_UPSTREAM}"
    -v "${CALL_STATE_DIR}:/opt/open-call"
    -v "${CALL_STATE_DIR}/config.yml:/opt/open-call/config.yml:ro"
    --log-opt max-size=20m
    --log-opt max-file=5
  )
  if [[ -n "${CALL_BINARY_MOUNT:-}" && -f "${CALL_BINARY_MOUNT}" ]]; then
    args+=(-v "${CALL_BINARY_MOUNT}:/usr/local/bin/open-call:ro")
  fi

  docker run "${args[@]}" "${image}"
  echo "Started ${container} from ${image}"
}

wait_switch_health() {
  echo "==> Switch health check..."
  local i
  for i in $(seq 1 30); do
    if curl --fail --silent --max-time 3 "${SWITCH_HEALTH_URL}" >/dev/null; then
      echo "Switch is healthy."
      return 0
    fi
    sleep 2
  done
  curl --fail --silent --max-time 5 "${SWITCH_HEALTH_URL}" >/dev/null
}

wait_call_health() {
  echo "==> Call / web health checks..."
  local i
  for i in $(seq 1 30); do
    if curl --fail --silent --max-time 3 "${CALL_HEALTH_URL}" >/dev/null \
      && curl --fail --silent --max-time 3 "${WEB_HEALTH_URL}" >/dev/null; then
      echo "Call API and web are healthy."
      return 0
    fi
    sleep 2
  done
  curl --fail --silent --max-time 5 "${CALL_HEALTH_URL}" >/dev/null
  curl --fail --silent --max-time 5 "${WEB_HEALTH_URL}" >/dev/null
}

ecs_switch() {
  require_env_file
  load_env
  export PULL=1

  local backup_dir="/opt/open-voip/backups/$(date +%Y%m%d-%H%M%S)"
  mkdir -p "${backup_dir}"

  if systemctl is-active --quiet open-switch 2>/dev/null; then
    echo "==> Stopping systemd open-switch..."
    systemctl stop open-switch
  fi
  if [[ -f /opt/open-switch/config.yml ]]; then
    cp -a /opt/open-switch/config.yml "${backup_dir}/open-switch-config.yml"
  fi

  run_switch
  wait_switch_health

  if systemctl list-unit-files open-switch.service >/dev/null 2>&1; then
    systemctl disable open-switch 2>/dev/null || true
    echo "Disabled systemd unit open-switch."
  fi
  echo "Done. Set IMAGE_TAG in ${ENV_FILE} and re-run: $0 ecs switch"
}

ecs_call() {
  require_env_file
  load_env
  export PULL=1

  local backup_dir="/opt/open-voip/backups/$(date +%Y%m%d-%H%M%S)"
  mkdir -p "${backup_dir}"

  if systemctl is-active --quiet open-call 2>/dev/null; then
    echo "==> Stopping systemd open-call..."
    systemctl stop open-call
  fi
  if [[ -f /opt/open-call/config.yml ]]; then
    cp -a /opt/open-call/config.yml "${backup_dir}/open-call-config.yml"
  fi

  run_call
  wait_call_health

  if systemctl list-unit-files open-call.service >/dev/null 2>&1; then
    systemctl disable open-call 2>/dev/null || true
    echo "Disabled systemd unit open-call."
  fi
  echo "Done. Set IMAGE_TAG in ${ENV_FILE} and re-run: $0 ecs call"
}

main() {
  require_docker

  local mode="${1:-ecs}"
  local target="${2:-all}"

  if [[ "${mode}" == "-h" || "${mode}" == "--help" ]]; then
    usage
    exit 0
  fi

  case "${mode}" in
    run)
      load_env
      case "${target}" in
        switch) run_switch ;;
        call) run_call ;;
        *)
          echo "unknown target: ${target}" >&2
          usage >&2
          exit 1
          ;;
      esac
      ;;
    ecs)
      case "${target}" in
        switch) ecs_switch ;;
        call) ecs_call ;;
        all)
          ecs_switch
          ecs_call
          echo "Full stack deploy finished."
          ;;
        *)
          echo "unknown target: ${target}" >&2
          usage >&2
          exit 1
          ;;
      esac
      ;;
    *)
      echo "unknown mode: ${mode}" >&2
      usage >&2
      exit 1
      ;;
  esac
}

main "$@"
