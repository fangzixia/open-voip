#!/usr/bin/env python3
"""Prepare ECS Docker runtime from existing host config.yml files.

Copies Switch/Call configs into a Compose bind-mount directory, sets HTTP listen
to 0.0.0.0, keeps database.dsn unchanged, and normalizes same-host URLs to
127.0.0.1. Does not start Postgres or rewrite credentials.
"""
import argparse
import json
import os
import re
import shutil
from pathlib import Path
def set_scalar(text, name, value):
    pattern = rf"(?m)^(\s*{re.escape(name)}:\s*).*$"
    text, count = re.subn(pattern, lambda m: m[1] + json.dumps(value, ensure_ascii=False), text)
    if count != 1:
        raise ValueError(f"expected one {name} field in configuration")
    return text


def set_server_listen(text, port):
    text, count = re.subn(
        r'(?m)^(server:[ \t]*\n[ \t]+listen:)[ \t]*[^\n]+',
        lambda m: m[1] + f' "0.0.0.0:{port}"',
        text,
    )
    if count != 1:
        raise ValueError("expected one server.listen field in Switch configuration")
    return text


def write_private(path, text, overwrite=False):
    path.parent.mkdir(parents=True, exist_ok=True)
    if overwrite and path.exists():
        path.write_text(text, encoding="utf-8", newline="\n")
        os.chmod(path, 0o600)
        return
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, "w", encoding="utf-8", newline="\n") as out:
        out.write(text)


parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("--runtime-dir", default="/opt/open-voip/runtime")
parser.add_argument("--env-file", default="/opt/open-voip/.env")
parser.add_argument("--switch-src", default="/opt/open-switch/config.yml")
parser.add_argument("--call-src", default="/opt/open-call/config.yml")
parser.add_argument("--switch-port", type=int, default=8082)
parser.add_argument("--call-port", type=int, default=8080)
parser.add_argument("--ghcr-registry", default="ghcr.io/fangzixia")
parser.add_argument("--image-tag", default="main")
parser.add_argument("--project-name", default="open-voip")
parser.add_argument("--force", action="store_true", help="overwrite runtime config.yml if present")
args = parser.parse_args()

runtime = Path(args.runtime_dir).resolve()
env_file = Path(args.env_file).resolve()
switch_dst = runtime / "open-switch" / "config.yml"
call_dst = runtime / "open-call" / "config.yml"

if not Path(args.switch_src).is_file():
    parser.error(f"Switch config not found: {args.switch_src}")
if not Path(args.call_src).is_file():
    parser.error(f"Call config not found: {args.call_src}")

for dst in (switch_dst, call_dst):
    if dst.exists() and not args.force:
        parser.error(f"{dst} exists; pass --force to overwrite")
if env_file.exists() and not args.force:
    parser.error(f"{env_file} exists; pass --force to overwrite")

switch = Path(args.switch_src).read_text(encoding="utf-8")
call = Path(args.call_src).read_text(encoding="utf-8")

switch = set_server_listen(switch, args.switch_port)
switch = set_scalar(switch, "events_callback_url",
                    f"http://127.0.0.1:{args.call_port}/api/v1/integration/switch/events")

call = set_scalar(call, "listen", f"0.0.0.0:{args.call_port}")
call = set_scalar(call, "switch_base_url", f"http://127.0.0.1:{args.switch_port}")
# Removed in current open-call; legacy configs fail YAML parse in new images.
call = re.sub(r"(?m)^  username:.*\n", "", call)
call = re.sub(r"(?m)^  password:.*\n", "", call)

switch_dst.parent.mkdir(parents=True, exist_ok=True)
call_dst.parent.mkdir(parents=True, exist_ok=True)

# Preserve recordings/logs under legacy /opt paths when migrating from systemd layout.
for name, src_root, dst_root in (
    ("open-switch", Path("/opt/open-switch"), runtime / "open-switch"),
    ("open-call", Path("/opt/open-call"), runtime / "open-call"),
):
    dst_root.mkdir(parents=True, exist_ok=True)
    if src_root.is_dir() and src_root.resolve() != dst_root.resolve():
        for item in src_root.iterdir():
            if item.name == "config.yml":
                continue
            target = dst_root / item.name
            if item.is_dir():
                if not target.exists():
                    shutil.copytree(item, target, dirs_exist_ok=True)
            elif not target.exists():
                shutil.copy2(item, target)

switch_dst.write_text(switch, encoding="utf-8", newline="\n")
call_dst.write_text(call, encoding="utf-8", newline="\n")
os.chmod(switch_dst, 0o600)
os.chmod(call_dst, 0o600)

uid = gid = 10001
if hasattr(os, "getuid") and os.getuid() == 0:
    for directory in (runtime / "open-switch", runtime / "open-call"):
        os.chown(directory, uid, gid)
        os.chown(directory / "config.yml", uid, gid)

def quoted(path):
    value = str(path).replace("\\", "/")
    if "'" in value or "\n" in value:
        raise ValueError("runtime path contains unsupported characters")
    return "'" + value + "'"

env = f"""COMPOSE_PROJECT_NAME={args.project_name}
GHCR_REGISTRY={args.ghcr_registry}
IMAGE_TAG={args.image_tag}
RUNTIME_DIR={quoted(runtime)}
SWITCH_STATE_DIR={quoted(runtime / 'open-switch')}
CALL_STATE_DIR={quoted(runtime / 'open-call')}
SWITCH_UID={uid}
SWITCH_GID={gid}
CALL_UID={uid}
CALL_GID={gid}
SWITCH_HEALTH_URL=http://127.0.0.1:{args.switch_port}/health
CALL_HEALTH_URL=http://127.0.0.1:{args.call_port}/health/ready
"""
write_private(env_file, env, overwrite=args.force)
print(f"Migrated configs to {runtime} and wrote {env_file}")
print("database.dsn unchanged. Ensure security group blocks public 8080/8082.")
