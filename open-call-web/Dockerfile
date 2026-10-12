FROM node:22-bookworm-slim AS build
WORKDIR /src/open-call-web
COPY open-call-web/package.json open-call-web/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci
COPY open-call-web/ ./
ENV VITE_API_BASE=""
RUN npm run build

FROM nginx:1.28-bookworm
RUN apt-get update && apt-get install -y --no-install-recommends curl ca-certificates \
    && rm -rf /var/lib/apt/lists/*
COPY open-call-web/docker/default.conf.template /etc/nginx/templates/default.conf.template
COPY --from=build /src/open-call-web/dist/ /usr/share/nginx/html/
ENV WEB_LISTEN=0.0.0.0:8088 CALL_HTTP_UPSTREAM=127.0.0.1:8080
