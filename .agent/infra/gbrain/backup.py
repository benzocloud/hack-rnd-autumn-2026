#!/usr/bin/env python3
"""Back up this PGLite owner only; do not open its DB concurrently with serve."""

import datetime
import fcntl
import subprocess
from pathlib import Path


def main():
    with open("/run/lock/hackrnd-gbrain-backup.lock", "w") as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        active = (
            subprocess.run(
                ["systemctl", "is-active", "--quiet", "hackrnd-gbrain.service"], check=False
            ).returncode
            == 0
        )
        if not active:
            raise SystemExit("Refusing backup: GBrain was not active")
        stamp = datetime.datetime.now(datetime.timezone.utc).strftime("%Y%m%dT%H%M%S%fZ")
        target = Path("/var/backups/hackrnd-gbrain") / f"brain-{stamp}.gbrain-backup"
        subprocess.run(["systemctl", "stop", "hackrnd-gbrain.service"], check=True)
        try:
            result = subprocess.run(
                [
                    "runuser",
                    "-u",
                    "hackrnd-gbrain",
                    "--",
                    "/opt/hackrnd-gbrain/bin/gbrain",
                    "backup",
                    "create",
                    "--output",
                    str(target),
                    "--json",
                ],
                cwd="/var/lib/hackrnd-gbrain",
                capture_output=True,
                text=True,
                timeout=480,
                check=False,
            )
            if result.returncode:
                raise SystemExit("GBrain backup failed; captured output is not published")
            print(f"Created private backup: {target.name}")
        finally:
            subprocess.run(["systemctl", "start", "hackrnd-gbrain.service"], check=True)


if __name__ == "__main__":
    main()
