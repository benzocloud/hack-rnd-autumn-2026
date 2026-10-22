#!/usr/bin/env python3
"""Configure only Hackrnd services and private credentials after bootstrap.py."""

import os
import pwd
import re
import secrets
import shutil
import subprocess
from pathlib import Path

HERE = Path(__file__).resolve().parent
BIN = "/opt/hackrnd-agent/bin/gbrain"
BRAIN = Path("/var/lib/hackrnd-gbrain")
REVIEW = Path("/var/lib/hackrnd-review")
SECRETS = Path("/etc/hackrnd-agent")


def brain(*args):
    result = subprocess.run(
        ["runuser", "-u", "hackrnd-gbrain", "--", BIN, *args],
        cwd=BRAIN,
        capture_output=True,
        text=True,
        check=True,
        env={**os.environ, "GBRAIN_NO_AUTOPILOT_INSTALL": "1", "GBRAIN_NO_REEMBED": "1"},
        timeout=120,
    )
    return result.stdout


def private_file(path, content, owner=None):
    descriptor = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(descriptor, "w") as out:
        out.write(content)
    if owner:
        user = pwd.getpwnam(owner)
        os.chown(path, user.pw_uid, user.pw_gid)


def main():
    if os.geteuid() != 0:
        raise SystemExit("Requires root")
    if "sse4_2" not in Path("/proc/cpuinfo").read_text().split():
        raise SystemExit("GBrain requires SSE4.2; service configuration was not applied")
    config = BRAIN / ".gbrain/config.json"
    active = (
        subprocess.run(["systemctl", "is-active", "--quiet", "hackrnd-gbrain.service"]).returncode
        == 0
    )
    token_specs = [
        ("reviewer", "read", SECRETS / "reviewer.token"),
        ("developer-globalarray", "read,write", SECRETS / "developer-globalarray.token"),
    ]
    if active and (not config.exists() or any(not path.exists() for _, _, path in token_specs)):
        raise SystemExit(
            "Existing service needs credential recovery; stop and inspect it before changing state"
        )
    if not config.exists():
        brain("init", "--pglite", "--no-embedding", "--non-interactive", "--db-only", "--json")
        print("Initialized a keyless PGLite brain", flush=True)
    for name, scopes, path in token_specs:
        if path.exists():
            continue
        output = brain("auth", "create", name, "--scopes", scopes)
        tokens = re.findall(r"\bgbrain_[A-Za-z0-9_-]+\b", output)
        if len(tokens) != 1:
            raise RuntimeError("Token creation returned unexpected output; inspect privately")
        private_file(path, tokens[0] + "\n")
        print(f"Created {name} credential with scopes {scopes}", flush=True)
    review_config = REVIEW / ".codex"
    review_config.mkdir(mode=0o700, exist_ok=True)
    review_config.chmod(0o700)
    user = pwd.getpwnam("hackrnd-review")
    os.chown(review_config, user.pw_uid, user.pw_gid)
    token_file = review_config / "gbrain.token"
    if not token_file.exists():
        private_file(token_file, (SECRETS / "reviewer.token").read_text(), "hackrnd-review")
    codex_config = review_config / "config.toml"
    if not codex_config.exists():
        private_file(
            codex_config, (HERE.parent / "runner/review-config.toml").read_text(), "hackrnd-review"
        )
    if not (SECRETS / "gbrain.env").exists():
        private_file(
            SECRETS / "gbrain.env",
            "GBRAIN_ADMIN_BOOTSTRAP_TOKEN=" + secrets.token_urlsafe(48) + "\n",
        )
    for name in [
        "hackrnd-gbrain.service",
        "hackrnd-gbrain-backup.service",
        "hackrnd-gbrain-backup.timer",
    ]:
        shutil.copyfile(HERE / name, Path("/etc/systemd/system") / name)
    Path("/opt/hackrnd-agent/gbrain").mkdir(exist_ok=True)
    shutil.copyfile(HERE / "backup.py", "/opt/hackrnd-agent/gbrain/backup.py")
    subprocess.run(["systemctl", "daemon-reload"], check=True)
    subprocess.run(
        ["systemctl", "enable", "--now", "hackrnd-gbrain.service", "hackrnd-gbrain-backup.timer"],
        check=True,
    )
    print("Configured Hackrnd GBrain and backup timer. Credentials were not printed.", flush=True)


if __name__ == "__main__":
    main()
