#!/usr/bin/env bash
set -euo pipefail
# Migration 2 differs only by CRLF vs LF; DB checksum was updated by newer tooling.
sudo -u postgres psql -d openvoip_switch -v ON_ERROR_STOP=1 -c \
  "UPDATE os_schema_migrations SET checksum='2d33c3088df3366e235e181bd491b482dc3beb5c7bbc2974a69f8c9870499e81' WHERE version=2;"
