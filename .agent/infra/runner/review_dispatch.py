#!/usr/bin/env python3
"""Root-owned sudo target: start exactly one fixed, unprivileged review service."""

import os
import signal
import subprocess
import sys
import uuid

COMMANDS = {
    "agent-review": "review/ssh_review.py",
    "agent-scope": "planning/ssh_plan.py",
    "agent-create-plan": "planning/ssh_plan.py",
    "agent-discuss-plan": "planning/ssh_plan.py",
    "agent-question": "conversation/ssh_conversation.py",
    "agent-discuss": "conversation/ssh_conversation.py",
}


def main():
    if (
        os.geteuid() != 0
        or len(sys.argv) != 1
        or os.environ.get("SSH_ORIGINAL_COMMAND") not in COMMANDS
    ):
        raise SystemExit("Only fixed review/planning commands are accepted")
    original = os.environ["SSH_ORIGINAL_COMMAND"]
    unit = "hackrnd-review-" + uuid.uuid4().hex
    command = [
        "systemd-run",
        "--quiet",
        "--wait",
        "--pipe",
        "--collect",
        "--unit=" + unit,
        "--slice=hackrnd-review.slice",
        "--uid=hackrnd-review",
        "--gid=hackrnd-review",
        "--property=NoNewPrivileges=yes",
        "--property=ProtectSystem=strict",
        "--property=ReadWritePaths=/var/lib/hackrnd-review",
        "--property=ProtectHome=yes",
        "--property=PrivateTmp=yes",
        "--property=ProtectKernelTunables=yes",
        "--property=ProtectKernelModules=yes",
        "--property=ProtectControlGroups=yes",
        "--property=RestrictSUIDSGID=yes",
        "--property=CapabilityBoundingSet=",
        "--property=RuntimeMaxSec=18min",
        "--property=TimeoutStopSec=15",
        "--property=KillMode=control-group",
        "--property=UMask=0077",
        "--setenv=HOME=/var/lib/hackrnd-review",
        "--setenv=SSH_ORIGINAL_COMMAND=" + original,
        "/usr/bin/python3",
        "-Es",
        "/opt/hackrnd-agent/" + COMMANDS[original],
    ]

    def cancel(signum, frame):
        subprocess.run(
            ["systemctl", "stop", unit], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL
        )
        raise SystemExit(128 + signum)

    for signum in (signal.SIGHUP, signal.SIGINT, signal.SIGTERM):
        signal.signal(signum, cancel)
    raise SystemExit(subprocess.call(command))


if __name__ == "__main__":
    main()
