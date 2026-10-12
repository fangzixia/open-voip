#!/usr/bin/env python3
"""Update os_schema_migrations.checksum to match SQL files in repo (metadata only)."""
import hashlib
import os
import subprocess
import sys

repo = os.environ.get("REPO", "/opt/open-voip/src-git")
sql_dir = os.path.join(repo, "open-switch/internal/store/migrate/sql")
out = "/tmp/fix-all-checksums.sql"
lines = []
for name in sorted(os.listdir(sql_dir)):
    if not name.endswith(".sql"):
        continue
    version = int(name.split("_")[0])
    raw = open(os.path.join(sql_dir, name), "rb").read()
    chk = hashlib.sha256(raw).hexdigest()
    lines.append(
        f"UPDATE os_schema_migrations SET checksum = '{chk}' "
        f"WHERE version = {version} AND name = '{name}';"
    )
open(out, "w", encoding="utf-8").write("\n".join(lines) + "\n")
print(f"Wrote {out} ({len(lines)} updates)")
subprocess.check_call(["sudo", "-u", "postgres", "psql", "-d", "openvoip_switch", "-f", out])
