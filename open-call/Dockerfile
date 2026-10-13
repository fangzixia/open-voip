ARG GO_IMAGE=golang:1.27.1-bookworm
ARG NODE_IMAGE=node:22-bookworm-slim
FROM ${GO_IMAGE} AS build
ARG GOPROXY=https://proxy.golang.org,direct
ENV CGO_ENABLED=0 GOPROXY=${GOPROXY}
WORKDIR /src/open-call
COPY open-call/go.mod open-call/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY open-call/ ./
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -o /out/open-call ./cmd/open-call

FROM ${NODE_IMAGE} AS web
ARG NPM_REGISTRY=https://registry.npmjs.org
ENV NPM_CONFIG_REGISTRY=${NPM_REGISTRY}
WORKDIR /src/open-call-web
COPY open-call-web/package.json open-call-web/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci
COPY open-call-web/ ./
ENV VITE_API_BASE=""
RUN npm run build

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl tzdata ffmpeg nginx gettext-base gosu \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --gid 10001 openvoip \
    && useradd --uid 10001 --gid openvoip --no-create-home openvoip \
    && mkdir -p /opt/open-call /etc/nginx/templates /etc/nginx/conf.d \
    && rm -f /etc/nginx/sites-enabled/default \
    && chown openvoip:openvoip /opt/open-call
COPY --from=build /out/open-call /usr/local/bin/open-call
COPY open-call-web/docker/default.conf.template /etc/nginx/templates/default.conf.template
COPY --from=web /src/open-call-web/dist/ /usr/share/nginx/html/
COPY open-call/docker/entrypoint.sh /usr/local/bin/open-call-entrypoint.sh
RUN chmod 755 /usr/local/bin/open-call-entrypoint.sh
WORKDIR /opt/open-call
ENV WEB_LISTEN=0.0.0.0:8088 CALL_HTTP_UPSTREAM=127.0.0.1:8080
ENTRYPOINT ["/usr/local/bin/open-call-entrypoint.sh"]
