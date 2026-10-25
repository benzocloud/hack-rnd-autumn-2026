#!/usr/bin/env python3
"""Tear down an isolated financial-e2e stack by marker/state.json only."""

from __future__ import annotations

import argparse
import json
import signal
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent
ARTIFACTS = ROOT / "artifacts"


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--state", type=Path, default=None)
    parser.add_argument("--suffix", default=None)
    args = parser.parse_args()

    state_path = args.state
    if state_path is None and args.suffix:
        state_path = ARTIFACTS / f"stack-{args.suffix}" / "state.json"
    if state_path is None:
        # newest state.json under artifacts/stack-*
        candidates = sorted(ARTIFACTS.glob("stack-*/state.json"), key=lambda p: p.stat().st_mtime, reverse=True)
        if not candidates:
            print("no stack state found", file=sys.stderr)
            return 2
        state_path = candidates[0]

    state = json.loads(state_path.read_text(encoding="utf-8"))
    project = state["project"]
    container = state.get("postgres_container")
    work = Path(state["work_dir"])
    marker = Path(state["marker"])
    if not marker.is_file() or project not in marker.read_text(encoding="utf-8"):
        print(f"refusing teardown: marker missing/mismatch for {project}", file=sys.stderr)
        return 3

    # Kill processes recorded in logs dir by reading pids from /proc is hard;
    # prefer SIGTERM to listeners on recorded ports via lsof if available.
    ports = state.get("ports") or {}
    for name, port in ports.items():
        if port is None:
            continue
        result = subprocess.run(
            ["lsof", "-ti", f"tcp:{port}"],
            text=True,
            capture_output=True,
            check=False,
        )
        for pid in result.stdout.split():
            try:
                Path(f"/proc/{pid}")  # noqa: not on mac
            except Exception:
                pass
            try:
                import os

                os.kill(int(pid), signal.SIGTERM)
                print(f"signaled pid {pid} for {name}:{port}")
            except ProcessLookupError:
                pass
            except PermissionError as exc:
                print(f"cannot signal {pid}: {exc}", file=sys.stderr)

    if container:
        inspect = subprocess.run(
            ["docker", "inspect", "-f", "{{index .Config.Labels \"saldo.financial-e2e\"}}", container],
            text=True,
            capture_output=True,
            check=False,
        )
        label = inspect.stdout.strip()
        if label == container:
            subprocess.run(["docker", "rm", "-f", container], check=False)
            print(f"removed {container}")
        else:
            print(f"refusing docker rm: label={label!r} expected={container!r}", file=sys.stderr)
            return 4

    print(json.dumps({"ok": True, "project": project, "work": str(work)}))
    return 0


if __name__ == "__main__":
    sys.exit(main())
