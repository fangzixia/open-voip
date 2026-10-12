#!/usr/bin/env bash
# Build Docker images from existing ECS binaries (same bits as systemd) and start Compose.
set -euo pipefail

SW="${SW_BIN_DIR:-/opt/open-switch}"
CA="${CA_BIN_DIR:-/opt/open-call}"
ROOT="${OPEN_VOIP_ROOT:-/opt/open-voip/src-git}"
ENV_FILE="${ENV_FILE:-/opt/open-voip/.env}"
TAG="${IMAGE_TAG:-ecs-runtime}"

cat >/tmp/Dockerfile.switch-runtime <<'EOF'
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl tzdata ffmpeg libsoxr0 libspandsp2 \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --gid 10001 openvoip && useradd --uid 10001 --gid openvoip --no-create-home openvoip \
    && mkdir -p /opt/open-switch && chown openvoip:openvoip /opt/open-switch
COPY open-switch /usr/local/bin/open-switch
RUN chmod 755 /usr/local/bin/open-switch
WORKDIR /opt/open-switch
USER 10001:10001
ENTRYPOINT ["/usr/local/bin/open-switch"]
CMD ["-config", "/opt/open-switch/config.yml"]
EOF

cat >/tmp/Dockerfile.call-runtime <<'EOF'
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl tzdata ffmpeg \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --gid 10001 openvoip && useradd --uid 10001 --gid openvoip --no-create-home openvoip \
    && mkdir -p /opt/open-call && chown openvoip:openvoip /opt/open-call
COPY open-call /usr/local/bin/open-call
RUN chmod 755 /usr/local/bin/open-call
WORKDIR /opt/open-call
USER 10001:10001
ENTRYPOINT ["/usr/local/bin/open-call"]
CMD ["-config", "/opt/open-call/config.yml"]
EOF

docker build -f /tmp/Dockerfile.switch-runtime -t "ghcr.io/fangzixia/open-voip-open-switch:${TAG}" "${SW}"
docker build -f /tmp/Dockerfile.call-runtime -t "ghcr.io/fangzixia/open-voip-open-call:${TAG}" "${CA}"

if grep -q '^IMAGE_TAG=' "${ENV_FILE}"; then
  sed -i "s/^IMAGE_TAG=.*/IMAGE_TAG=${TAG}/" "${ENV_FILE}"
else
  echo "IMAGE_TAG=${TAG}" >>"${ENV_FILE}"
fi

systemctl stop open-switch open-call 2>/dev/null || true

cd "${ROOT}"
docker compose --env-file "${ENV_FILE}" -f compose.yml up -d --force-recreate open-switch open-call

for _ in $(seq 1 30); do
  if curl -sf -m 3 http://127.0.0.1:8082/health >/dev/null \
    && curl -sf -m 3 http://127.0.0.1:8080/health/ready >/dev/null; then
    echo "Switch and Call are healthy."
    break
  fi
  sleep 2
done

curl -sf -m 5 http://127.0.0.1:8082/health
echo
curl -sf -m 5 http://127.0.0.1:8080/health/ready
echo
docker ps --format 'table {{.Names}}\t{{.Status}}\t{{.Image}}'
systemctl disable open-switch open-call 2>/dev/null || true
echo "Done. IMAGE_TAG=${TAG}"
