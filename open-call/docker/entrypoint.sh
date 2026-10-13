#!/usr/bin/env bash
set -euo pipefail

WEB_LISTEN="${WEB_LISTEN:-0.0.0.0:8088}"
CALL_HTTP_UPSTREAM="${CALL_HTTP_UPSTREAM:-127.0.0.1:8080}"
export WEB_LISTEN CALL_HTTP_UPSTREAM

template="/etc/nginx/templates/default.conf.template"
conf="/etc/nginx/conf.d/default.conf"
if [[ -f "${template}" ]]; then
  envsubst '${WEB_LISTEN} ${CALL_HTTP_UPSTREAM}' <"${template}" >"${conf}"
fi

nginx

exec gosu openvoip:openvoip /usr/local/bin/open-call -config /opt/open-call/config.yml "$@"
