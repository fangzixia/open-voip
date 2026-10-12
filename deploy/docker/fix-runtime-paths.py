#!/usr/bin/env python3
"""Adjust log/recording paths in runtime configs for Docker bind mounts."""
from pathlib import Path

pairs = (
    ("/opt/open-voip/runtime/open-switch/config.yml", "/opt/open-switch", "/opt/open-voip/runtime/open-switch"),
    ("/opt/open-voip/runtime/open-call/config.yml", "/opt/open-call", "/opt/open-voip/runtime/open-call"),
)
for path, old, new in pairs:
    p = Path(path)
    text = p.read_text(encoding="utf-8")
    p.write_text(text.replace(old, new), encoding="utf-8", newline="\n")
    print(f"updated {path}")
