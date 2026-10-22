#!/usr/bin/env python3
"""Quiesce only our GBrain, create a native full backup, always restart it."""

import datetime
import fcntl
import subprocess
from pathlib import Path


def main():
    with open("/run/lock/hackrnd-gbrain-backup.lock", "w") as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        stamp = datetime.datetime.now(datetime.timezone.utc).strftime("%Y%m%dT%H%M%S%fZ")
        target = Path("/var/backups/hackrnd-gbrain") / f"brain-{stamp}.gbrain-backup"
        subprocess.run(["systemctl", "stop", "hackrnd-gbrain.service"], check=True)
        try:
            subprocess.run(
                [
                    "runuser",
                    "-u",
                    "hackrnd-gbrain",
                    "--",
                    "/opt/hackrnd-agent/bin/gbrain",
                    "backup",
                    "create",
                    "--output",
                    str(target),
                    "--json",
                ],
                check=True,
                cwd="/var/lib/hackrnd-gbrain",
            )
        finally:
            subprocess.run(["systemctl", "start", "hackrnd-gbrain.service"], check=True)


if __name__ == "__main__":
    main()
