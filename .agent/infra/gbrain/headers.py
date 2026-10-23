#!/usr/bin/env python3
"""Codex HTTP-header helper. Its credential output belongs only to the MCP client."""

import json
import stat
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]


def main():
    token_file = ROOT / ".agent/local/gbrain-developer.token"
    if stat.S_IMODE(token_file.stat().st_mode) & 0o077:
        raise SystemExit("GBrain credential must be private (chmod 600)")
    token = token_file.read_text().strip()
    if not token.startswith("gbrain_") or any(char.isspace() for char in token):
        raise SystemExit("Invalid GBrain credential file")
    print(json.dumps({"Authorization": "Bearer " + token}))


if __name__ == "__main__":
    main()
