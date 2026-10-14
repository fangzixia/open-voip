#!/usr/bin/env bash
# Pack open-call image using existing open-call-web/dist and docker go build.
set -euo pipefail
ROOT="${OPEN_VOIP_ROOT:-/opt/open-voip/src-git}"
TAG="${IMAGE_TAG:-main}"
REGISTRY="${GHCR_REGISTRY:-ghcr.io/fangzixia}"
IMAGE="${REGISTRY}/open-voip-open-call:${TAG}"

cd "${ROOT}"
test -f open-call-web/dist/index.html || { echo "missing open-call-web/dist; build or upload first" >&2; exit 1; }

docker build -f open-call/Dockerfile --target build \
  --build-arg GOPROXY="${GOPROXY:-https://goproxy.cn,direct}" \
  -t ov-call-go-build "${ROOT}"

cid="$(docker create ov-call-go-build)"
docker cp "${cid}:/out/open-call" /tmp/open-call-bin
docker rm "${cid}"
chmod 755 /tmp/open-call-bin
cp /tmp/open-call-bin "${ROOT}/call-bin"

docker build -f deploy/docker/Dockerfile.call-pack -t "${IMAGE}" "${ROOT}"
rm -f "${ROOT}/call-bin"
echo "Built ${IMAGE}"
