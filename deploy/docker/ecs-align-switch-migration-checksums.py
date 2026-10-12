#!/usr/bin/env python3
"""Align os_schema_migrations.checksum with release SQL files (CRLF) for prod binary."""
import hashlib
import re
import subprocess
from pathlib import Path

SQL_DIRS = [
    Path("/opt/open-voip/releases/20261006T140219Z/open-switch/internal/store/migrate/sql"),
    Path("/opt/open-voip/src-git/open-switch/internal/store/migrate/sql"),
]
DB = "openvoip_switch"


def sha256_file(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def find_sql(version: int, name: str) -> Path | None:
    for root in SQL_DIRS:
        p = root / name
        if p.is_file():
            return p
    for root in SQL_DIRS:
        for p in sorted(root.glob("*.sql")):
            m = re.match(r"^(\d+)_", p.name)
            if m and int(m.group(1)) == version:
                return p
    return None


def main() -> None:
    out = subprocess.check_output(
        ["sudo", "-u", "postgres", "psql", "-d", DB, "-At", "-c",
         "SELECT version, name FROM os_schema_migrations ORDER BY version"],
        text=True,
    )
    for line in out.strip().splitlines():
        version_s, name = line.split("|", 1)
        version = int(version_s)
        sql = find_sql(version, name)
        if not sql:
            print(f"skip version {version}: no sql file for {name}")
            continue
        # Prefer release tree bytes (CRLF) when present.
        release = SQL_DIRS[0] / name
        path = release if release.is_file() else sql
        checksum = sha256_file(path)
        subprocess.run(
            [
                "sudo", "-u", "postgres", "psql", "-d", DB, "-v", "ON_ERROR_STOP=1",
                "-c", f"UPDATE os_schema_migrations SET checksum='{checksum}' WHERE version={version};",
            ],
            check=True,
        )
        print(f"version {version} -> {checksum[:12]}... ({path})")


if __name__ == "__main__":
    main()
