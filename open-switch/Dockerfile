ARG GO_IMAGE=golang:1.27.1-bookworm
FROM ${GO_IMAGE} AS build
ARG GOPROXY=https://proxy.golang.org,direct
ENV CGO_ENABLED=1 GOPROXY=${GOPROXY}
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential pkg-config libsoxr-dev libspandsp-dev \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /src
COPY go.mod go.sum ./
COPY third_party/media-sdk ./third_party/media-sdk
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . ./
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    bash tools/build-linux.sh /out

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl tzdata ffmpeg libsoxr0 libspandsp2 \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --gid 10001 openvoip \
    && useradd --uid 10001 --gid openvoip --no-create-home openvoip \
    && mkdir -p /opt/open-switch \
    && chown openvoip:openvoip /opt/open-switch
COPY --from=build /out/open-switch /usr/local/bin/open-switch
COPY --from=build /out/ /usr/share/open-voip/build/
WORKDIR /opt/open-switch
USER 10001:10001
ENTRYPOINT ["/usr/local/bin/open-switch"]
CMD ["-config", "/opt/open-switch/config.yml"]
