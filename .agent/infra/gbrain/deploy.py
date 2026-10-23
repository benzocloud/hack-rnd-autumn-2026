#!/usr/bin/env python3
"""Provision the dedicated GBrain host; never use the runner/Fadwix bootstrap."""

import hashlib
import json
import os
import platform
import pwd
import re
import secrets
import shutil
import subprocess
import urllib.request
from pathlib import Path

HERE = Path(__file__).resolve().parent
ROOT = Path("/opt/hackrnd-gbrain")
BRAIN = Path("/var/lib/hackrnd-gbrain")
SECRETS = Path("/etc/hackrnd-gbrain")
USER = "hackrnd-gbrain"


def run(*args):
    subprocess.run(args, check=True)


def private_file(path, content):
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, "w") as stream:
        stream.write(content)


def brain(*args):
    result = subprocess.run(
        ["runuser", "-u", USER, "--", str(ROOT / "bin/gbrain"), *args],
        cwd=BRAIN,
        capture_output=True,
        text=True,
        timeout=300,
        check=False,
        env={
            **os.environ,
            "GBRAIN_HOME": str(BRAIN),
            "GBRAIN_NO_AUTOPILOT_INSTALL": "1",
            "GBRAIN_NO_REEMBED": "1",
        },
    )
    if result.returncode:
        raise SystemExit(f"GBrain {args[0]} failed; private output is not published")
    return result.stdout


def main():
    if os.geteuid() != 0 or platform.machine() != "x86_64":
        raise SystemExit("Requires root on the dedicated x86_64 GBrain host")
    if "sse4_2" not in Path("/proc/cpuinfo").read_text().split():
        raise SystemExit("CPU lacks SSE4.2; nothing was installed")
    if not shutil.which("caddy"):
        raise SystemExit("Install the distribution caddy package first")
    specs = json.loads((HERE / "versions.json").read_text())
    release = ROOT / "releases" / specs["version"]
    release.mkdir(parents=True, exist_ok=True)
    binary = release / "gbrain"
    if not binary.exists():
        temporary = release / "gbrain.download"
        digest = hashlib.sha256()
        with (
            urllib.request.urlopen(specs["url"], timeout=90) as source,
            temporary.open("wb") as out,
        ):
            while block := source.read(1024 * 1024):
                digest.update(block)
                out.write(block)
        if digest.hexdigest() != specs["sha256"]:
            temporary.unlink()
            raise SystemExit("GBrain artifact digest mismatch")
        temporary.chmod(0o755)
        temporary.rename(binary)
    if hashlib.sha256(binary.read_bytes()).hexdigest() != specs["sha256"]:
        raise SystemExit("Existing GBrain artifact differs from pinned release")
    (ROOT / "bin").mkdir(exist_ok=True)
    link = ROOT / "bin/gbrain"
    if not link.exists():
        link.symlink_to(binary)
    elif link.resolve() != binary:
        raise SystemExit("Existing binary pin differs; use an explicit upgrade plan")
    try:
        account = pwd.getpwnam(USER)
    except KeyError:
        run("useradd", "--system", "--home-dir", str(BRAIN), "--shell", "/usr/sbin/nologin", USER)
        account = pwd.getpwnam(USER)
    for directory in (BRAIN, Path("/var/backups/hackrnd-gbrain")):
        directory.mkdir(mode=0o700, exist_ok=True)
        directory.chmod(0o700)
        os.chown(directory, account.pw_uid, account.pw_gid)
    SECRETS.mkdir(mode=0o700, exist_ok=True)
    SECRETS.chmod(0o700)
    active = (
        subprocess.run(
            ["systemctl", "is-active", "--quiet", "hackrnd-gbrain.service"], check=False
        ).returncode
        == 0
    )
    config = BRAIN / ".gbrain/config.json"
    tokens = [("developer-globalarray", "read,write"), ("reviewer", "read")]
    if active and (
        not config.exists() or any(not (SECRETS / f"{name}.token").exists() for name, _ in tokens)
    ):
        raise SystemExit("Active PGLite owner: credential recovery needs a maintenance window")
    if not config.exists():
        brain("init", "--pglite", "--no-embedding", "--non-interactive", "--db-only", "--json")
    for name, scopes in tokens:
        path = SECRETS / f"{name}.token"
        if not path.exists():
            matches = re.findall(
                r"\bgbrain_[A-Za-z0-9_-]+\b", brain("auth", "create", name, "--scopes", scopes)
            )
            if len(matches) != 1:
                raise SystemExit("Unexpected credential response; inspect privately")
            private_file(path, matches[0] + "\n")
    if not (SECRETS / "service.env").exists():
        private_file(
            SECRETS / "service.env",
            "GBRAIN_ADMIN_BOOTSTRAP_TOKEN=" + secrets.token_urlsafe(48) + "\n",
        )
    for name in (
        "hackrnd-gbrain.service",
        "hackrnd-gbrain-backup.service",
        "hackrnd-gbrain-backup.timer",
    ):
        shutil.copyfile(HERE / name, Path("/etc/systemd/system") / name)
    deployment = ROOT / "deployment"
    deployment.mkdir(exist_ok=True)
    for name in ("backup.py", "versions.json"):
        shutil.copyfile(HERE / name, deployment / name)
    caddyfile = Path("/etc/caddy/Caddyfile")
    original = caddyfile.read_text()
    managed = (HERE / "Caddyfile").read_text()
    if original != managed:
        package_files = subprocess.check_output(
            ["dpkg-query", "-W", "-f=${Conffiles}", "caddy"], text=True
        )
        packaged_digest = next(
            (
                line.split()[1]
                for line in package_files.splitlines()
                if line.split() and line.split()[0] == str(caddyfile)
            ),
            None,
        )
        # dpkg's MD5 detects a modified conffile; release authenticity uses SHA-256 above.
        if hashlib.md5(original.encode(), usedforsecurity=False).hexdigest() != packaged_digest:
            raise SystemExit("Existing custom Caddyfile: refusing to replace other sites")
    run("caddy", "validate", "--config", str(HERE / "Caddyfile"), "--adapter", "caddyfile")
    if not (deployment / "Caddyfile.before").exists():
        shutil.copyfile(caddyfile, deployment / "Caddyfile.before")
    shutil.copyfile(HERE / "Caddyfile", caddyfile)
    run("systemctl", "daemon-reload")
    run("systemctl", "enable", "--now", "hackrnd-gbrain.service", "hackrnd-gbrain-backup.timer")
    run("systemctl", "reload", "caddy.service")
    print("GBrain service enabled. Credentials remain in protected files; verify HTTPS and MCP.")


if __name__ == "__main__":
    main()
