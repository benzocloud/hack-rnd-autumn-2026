#!/usr/bin/env python3
"""Install only this project's MCP configuration; credential stays outside Git."""

import argparse
import json
import os
import shlex
import sys
from pathlib import Path

import tomllib

ROOT = Path(__file__).resolve().parents[3]
URL = "https://gbrain.brokebyte.me/mcp"


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--token-file", type=Path, required=True)
    args = parser.parse_args()
    token = args.token_file.read_text().strip()
    if not token.startswith("gbrain_") or any(char.isspace() for char in token):
        raise SystemExit("Invalid token file")
    config = ROOT / ".codex/config.toml"
    original = config.read_text() if config.exists() else ""
    parsed = tomllib.loads(original)
    if "gbrain" in parsed.get("mcp_servers", {}):
        raise SystemExit("Project already has GBrain configured; preserve it and inspect privately")
    local = ROOT / ".agent/local"
    local.mkdir(mode=0o700, exist_ok=True)
    target = local / "gbrain-developer.token"
    if target.exists():
        if target.read_text().strip() != token:
            raise SystemExit("Existing token differs; explicit rotation required")
        target.chmod(0o600)
    else:
        fd = os.open(target, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        with os.fdopen(fd, "w") as stream:
            stream.write(token + "\n")
    helper = shlex.join([sys.executable, str(Path(__file__).with_name("headers.py"))])
    fragment = (
        "\n[mcp_servers.gbrain]\n"
        f"url = {json.dumps(URL)}\n"
        f"http_headers_helper = {json.dumps(helper)}\n"
        "enabled = true\nrequired = false\nstartup_timeout_sec = 30\ntool_timeout_sec = 60\n"
        'enabled_tools = ["recall", "remember", "entity", "context_pack", "delta"]\n'
    )
    tomllib.loads(original + fragment)
    config.parent.mkdir(mode=0o700, exist_ok=True)
    fd = os.open(config, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with os.fdopen(fd, "w") as stream:
        stream.write(original + fragment)
    config.chmod(0o600)
    print("Project MCP configured. Start a fresh Codex session; credential was not printed.")


if __name__ == "__main__":
    main()
