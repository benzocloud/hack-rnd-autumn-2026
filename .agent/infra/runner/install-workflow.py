#!/usr/bin/env python3
"""Install only the fixed Hackrnd review SSH entrypoint; no GitHub runner daemon."""
import argparse
import base64
import os
from pathlib import Path
import pwd
import shutil
import subprocess


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("public_key", type=Path)
    args = parser.parse_args()
    if os.geteuid() != 0:
        raise SystemExit("Requires root")
    key = args.public_key.read_text().strip().split()
    if len(key) < 2 or key[0] != "ssh-ed25519" or len(base64.b64decode(key[1], validate=True)) != 51:
        raise ValueError("Expected one Ed25519 public key")
    root, home = Path("/opt/hackrnd-agent"), Path("/var/lib/hackrnd-review")
    here = Path(__file__).resolve().parent
    user = pwd.getpwnam("hackrnd-review")
    for directory, names in {"review": ("ssh_review.py", "review_policy.py"),
                             "runner": ("review_dispatch.py", "job.py"),
                             "planning": ("ssh_plan.py", "plan_policy.py")}.items():
        target_dir = root / directory
        target_dir.mkdir(exist_ok=True)
        os.chown(target_dir, 0, 0); target_dir.chmod(0o755)
        for name in names:
            target = target_dir / name
            shutil.copyfile(here.parent / directory / name, target)
            target.chmod(0o644)
            os.chown(target, 0, 0)
    profile = home / ".codex/workflow.config.toml"
    shutil.copyfile(here / "workflow.config.toml", profile)
    profile.chmod(0o600)
    os.chown(profile, user.pw_uid, user.pw_gid)
    ssh = home / ".ssh"
    ssh.mkdir(exist_ok=True)
    os.chown(ssh, 0, 0); ssh.chmod(0o755)
    keys = ssh / "authorized_keys"
    old_flat_forced = 'restrict,command="/usr/bin/sudo -n /usr/bin/python3 -Es /opt/hackrnd-agent/review_dispatch.py" '
    old_forced = 'restrict,command="/usr/bin/python3 -Es /opt/hackrnd-agent/ssh_review.py" '
    forced = 'restrict,command="/usr/bin/sudo -n /usr/bin/python3 -Es /opt/hackrnd-agent/runner/review_dispatch.py" '
    line = forced + key[0] + " " + key[1] + " hackrnd-github-review\n"
    existing = keys.read_text() if keys.exists() else ""
    legacy = old_forced + key[0] + " " + key[1] + " hackrnd-github-review\n"
    if existing and existing not in (line, legacy, old_flat_forced + key[0] + " " + key[1] + " hackrnd-github-review\n"):
        raise ValueError("Existing SSH keys differ; refusing to overwrite")
    keys.write_text(line)
    os.chown(keys, 0, 0); keys.chmod(0o644)
    subprocess.run(["usermod", "--shell", "/bin/sh", "hackrnd-review"], check=True)
    sudo = Path("/etc/sudoers.d/hackrnd-agent-review")
    sudo.write_text('Defaults:hackrnd-review env_keep += "SSH_ORIGINAL_COMMAND"\n'
                    'hackrnd-review ALL=(root) NOPASSWD: /usr/bin/python3 -Es /opt/hackrnd-agent/runner/review_dispatch.py\n')
    sudo.chmod(0o440)
    subprocess.run(["visudo", "-cf", str(sudo)], check=True)
    shutil.copyfile(here / "hackrnd-review.slice", "/etc/systemd/system/hackrnd-review.slice")
    subprocess.run(["systemctl", "daemon-reload"], check=True)
    print("Fixed review/planning SSH key and per-review-user resource limits installed; auth and Fadwix untouched.")


if __name__ == "__main__":
    main()
