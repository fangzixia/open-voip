ARG GO_IMAGE=golang:1.27.1-bookworm
FROM ${GO_IMAGE} AS build
ARG GOPROXY=https://proxy.golang.org,direct
ENV CGO_ENABLED=0 GOPROXY=${GOPROXY}
WORKDIR /src/open-call
COPY open-call/go.mod open-call/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY open-call/ ./
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -o /out/open-call ./cmd/open-call

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl tzdata ffmpeg \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --gid 10001 openvoip \
    && useradd --uid 10001 --gid openvoip --no-create-home openvoip \
    && mkdir -p /opt/open-call \
    && chown openvoip:openvoip /opt/open-call
COPY --from=build /out/open-call /usr/local/bin/open-call
WORKDIR /opt/open-call
USER 10001:10001
ENTRYPOINT ["/usr/local/bin/open-call"]
CMD ["-config", "/opt/open-call/config.yml"]
