#!/usr/bin/env bash
# Build open-call runtime image on ECS when npm inside Docker is unreliable.
set -euo pipefail

ROOT="${OPEN_VOIP_ROOT:-/opt/open-voip/src-git}"
TAG="${IMAGE_TAG:-main}"
REGISTRY="${GHCR_REGISTRY:-ghcr.io/fangzixia}"
IMAGE="${REGISTRY}/open-voip-open-call:${TAG}"

cd "${ROOT}"

echo "==> npm build (host)..."
bash -lc 'source /root/.nvm/nvm.sh && cd "'"${ROOT}"'/open-call-web" && rm -rf node_modules && npm install && npm run build'

echo "==> go build (docker stage)..."
docker build -f open-call/Dockerfile --target build \
  --build-arg GOPROXY="${GOPROXY:-https://goproxy.cn,direct}" \
  -t ov-call-go-build "${ROOT}"

cid="$(docker create ov-call-go-build)"
docker cp "${cid}:/out/open-call" /tmp/open-call-bin
docker rm "${cid}"
chmod 755 /tmp/open-call-bin
cp /tmp/open-call-bin "${ROOT}/call-bin"

echo "==> runtime image..."
docker build -f - -t "${IMAGE}" "${ROOT}" <<EOF
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \\
    ca-certificates curl tzdata ffmpeg nginx gettext-base gosu \\
    && rm -rf /var/lib/apt/lists/* \\
    && groupadd --gid 10001 openvoip \\
    && useradd --uid 10001 --gid openvoip --no-create-home openvoip \\
    && mkdir -p /opt/open-call /etc/nginx/templates /etc/nginx/conf.d \\
    && rm -f /etc/nginx/sites-enabled/default \\
    && chown openvoip:openvoip /opt/open-call
COPY open-call-web/docker/default.conf.template /etc/nginx/templates/default.conf.template
COPY open-call/docker/entrypoint.sh /usr/local/bin/open-call-entrypoint.sh
COPY open-call-web/dist /usr/share/nginx/html
COPY call-bin /usr/local/bin/open-call
RUN chmod 755 /usr/local/bin/open-call-entrypoint.sh /usr/local/bin/open-call
WORKDIR /opt/open-call
ENV WEB_LISTEN=0.0.0.0:8088 CALL_HTTP_UPSTREAM=127.0.0.1:8080
ENTRYPOINT ["/usr/local/bin/open-call-entrypoint.sh"]
EOF

rm -f "${ROOT}/call-bin"
echo "Built ${IMAGE}"
