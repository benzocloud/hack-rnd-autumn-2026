#!/usr/bin/env python3
"""Prepare a private Codex home. Does not start a PR runner or publish reviews."""

import os
import pwd
import shutil
import subprocess
from pathlib import Path


def main():
    if os.geteuid() != 0:
        raise SystemExit("Requires root")
    home = Path("/var/lib/hackrnd-review")
    user = pwd.getpwnam("hackrnd-review")
    config = home / ".codex"
    config.mkdir(mode=0o700, exist_ok=True)
    config.chmod(0o700)
    os.chown(config, user.pw_uid, user.pw_gid)
    destination = config / "config.toml"
    if not destination.exists():
        shutil.copyfile(Path(__file__).with_name("review-config.toml"), destination)
        destination.chmod(0o600)
        os.chown(destination, user.pw_uid, user.pw_gid)
    for name in ["checks", "jobs"]:
        folder = home / name
        folder.mkdir(mode=0o700, exist_ok=True)
        os.chown(folder, user.pw_uid, user.pw_gid)
    restriction = Path("/proc/sys/kernel/apparmor_restrict_unprivileged_userns")
    if restriction.exists() and restriction.read_text().strip() == "1":
        profile = Path("/etc/apparmor.d/hackrnd-codex-bwrap")
        shutil.copyfile(Path(__file__).with_name("apparmor-hackrnd-bwrap"), profile)
        profile.chmod(0o644)
        subprocess.run(["apparmor_parser", "--replace", str(profile)], check=True)
    print("Review configuration prepared. GBrain is optional and disabled by default (ADR-010).")


if __name__ == "__main__":
    main()
