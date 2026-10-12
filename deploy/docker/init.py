#!/usr/bin/env python3
"""Initialize a fresh local Docker install without printing any credentials.

ECS production should use migrate-ecs-config.py with existing host Postgres DSN instead.
HTTP listen is 0.0.0.0; Switch/Call URLs for same-host traffic use 127.0.0.1.
"""
import argparse
import json
import os
from pathlib import Path
import re
import secrets
from urllib.parse import urlparse


def set_scalar(text, name, value):
    pattern = rf"(?m)^(\s*{re.escape(name)}:\s*).*$"
    text, count = re.subn(pattern, lambda m: m[1] + json.dumps(value, ensure_ascii=False), text)
    if count != 1:
        raise ValueError(f"expected one {name} field in example configuration")
    return text


def write_private(path, text):
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, "w", encoding="utf-8", newline="\n") as out:
        out.write(text)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--runtime-dir", default="runtime")
    parser.add_argument("--env-file", default=".env")
    parser.add_argument("--switch-port", type=int, default=8082)
    parser.add_argument("--call-port", type=int, default=8080)
    parser.add_argument("--database-port", type=int, default=15432)
    parser.add_argument("--web-port", type=int, default=8088)
    parser.add_argument("--public-url", default="http://localhost:8088")
    parser.add_argument("--project-name", default="open-voip")
    args = parser.parse_args()
    root = Path(__file__).resolve().parents[2]
    runtime = Path(args.runtime_dir).resolve()
    env_file = Path(args.env_file).resolve()
    for port in (args.switch_port, args.call_port, args.database_port, args.web_port):
        if not 1024 <= port <= 65535:
            parser.error("ports must be between 1024 and 65535")
    if len({args.switch_port, args.call_port, args.database_port, args.web_port}) != 4:
        parser.error("each service needs a distinct port")
    if not re.fullmatch(r"[a-z0-9][a-z0-9_-]*", args.project_name):
        parser.error("invalid Compose project name")
    if urlparse(args.public_url).scheme not in ("http", "https"):
        parser.error("public URL must use http or https")
    expected = [runtime / "open-switch/config.yml", runtime / "open-call/config.yml", env_file,
                runtime / "secrets/postgres_password", runtime / "bootstrap-credentials.json"]
    if any(path.exists() for path in expected):
        parser.error("runtime files already exist; refusing to overwrite configuration or credentials")
    for path in (runtime, runtime / "open-switch", runtime / "open-call", runtime / "secrets", env_file.parent):
        path.mkdir(parents=True, exist_ok=True)
    password = secrets.token_urlsafe(32)
    admin_password, agent_password = secrets.token_urlsafe(24), secrets.token_urlsafe(24)
    switch = (root / "open-switch/deploy/config.example.yml").read_text(encoding="utf-8")
    call = (root / "open-call/deploy/config.example.yml").read_text(encoding="utf-8")
    switch, count = re.subn(r'(?m)^(server:[ \t]*\n[ \t]+listen:)[ \t]*[^\n]+',
                            lambda m: m[1] + f' "0.0.0.0:{args.switch_port}"', switch)
    if count != 1:
        raise ValueError("expected one server.listen field in Switch example")
    switch = set_scalar(switch, "events_callback_url", f"http://127.0.0.1:{args.call_port}/api/v1/integration/switch/events")
    call = set_scalar(call, "listen", f"0.0.0.0:{args.call_port}")
    call = set_scalar(call, "switch_base_url", f"http://127.0.0.1:{args.switch_port}")
    for name, text in (("switch", switch), ("call", call)):
        dsn = f"host=127.0.0.1 user=openvoip password={password} dbname=openvoip_{name} port={args.database_port} sslmode=disable"
        text = set_scalar(text, "dsn", dsn)
        if name == "call":
            text = set_scalar(text, "signing_key", secrets.token_urlsafe(48))
            text = set_scalar(text, "admin_password", admin_password)
            text = set_scalar(text, "agent_password", agent_password)
            text = set_scalar(text, "guest_base_url", args.public_url.rstrip("/") + "/guest")
            text = re.sub(r'(allowed_origins:\s*\n)\s*-\s*"\*"', lambda m: m[1] + "    - " + json.dumps(args.public_url.rstrip("/")), text, count=1)
        write_private(runtime / f"open-{name}/config.yml", text)
    write_private(runtime / "secrets/postgres_password", password + "\n")
    write_private(runtime / "bootstrap-credentials.json", json.dumps({"admin": admin_password, "agent": agent_password}, indent=2) + "\n")
    # A Linux host needs writable bind directories and readable private config.
    uid, gid = (os.getuid(), os.getgid()) if hasattr(os, "getuid") else (10001, 10001)
    if uid == 0:
        uid = gid = 10001
        for directory in (runtime / "open-switch", runtime / "open-call"):
            os.chown(directory, uid, gid)
            os.chown(directory / "config.yml", uid, gid)
    def quoted(path):
        # Single-quoted Compose values preserve dollar signs in local paths.
        value = str(path).replace("\\", "/")
        if "'" in value or "\n" in value:
            raise ValueError("runtime path contains unsupported characters")
        return "'" + value + "'"
    env = f"""COMPOSE_PROJECT_NAME={args.project_name}
IMAGE_TAG=local
RUNTIME_DIR={quoted(runtime)}
SWITCH_STATE_DIR={quoted(runtime / 'open-switch')}
CALL_STATE_DIR={quoted(runtime / 'open-call')}
SWITCH_UID={uid}
SWITCH_GID={gid}
CALL_UID={uid}
CALL_GID={gid}
DATABASE_PORT={args.database_port}
SWITCH_HEALTH_URL=http://127.0.0.1:{args.switch_port}/health
CALL_HEALTH_URL=http://127.0.0.1:{args.call_port}/health/ready
CALL_HTTP_UPSTREAM=127.0.0.1:{args.call_port}
WEB_LISTEN=0.0.0.0:{args.web_port}
WEB_HEALTH_URL=http://127.0.0.1:{args.web_port}/container-health
"""
    write_private(env_file, env)
    print(f"Initialized {runtime}. Credentials are in bootstrap-credentials.json; values were not printed.")
    print("Review public URL, SIP external IP/ICE and origins before enabling public calls.")


if __name__ == "__main__":
    main()
